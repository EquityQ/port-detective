// Package core 是 port-detective 的核心业务层：负责本地端口的占用探测与
// 进程体检。本包只依赖标准库与 gopsutil，严禁依赖 cobra 或 mcp-go，
// 以保证 CLI 层与 MCP 层可以各自独立复用同一套能力。
package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime"
	"sort"
	"sync"
	"syscall"
	"time"

	// 标准库的 net 负责 TCP 连通性探测；gopsutil 的同名子包负责系统连接表，
	// 这里用别名 gnet 区分，避免包名冲突。
	gnet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

// ============================ 常量与类型契约 ============================

// 探测协议：与 gopsutil 无关，仅在 core 包内使用的稳定对外契约。
const (
	// ProtocolTCP 表示 TCP 协议。
	ProtocolTCP = "tcp"
	// ProtocolUDP 表示 UDP 协议。
	ProtocolUDP = "udp"

	// DefaultDialTimeout 是单次 TCP 连通性探测的超时时间。
	// 取较小值是为了让 1-65535 的全量扫描依然能在秒级返回。
	DefaultDialTimeout = 300 * time.Millisecond

	// DefaultWorkers 是并发扫描的默认协程数量。
	DefaultWorkers = 64

	// maxAutoWorkers 用于限制「按 CPU 核数自动推导」时的协程上限，
	// 避免在大核数机器上创建过多无谓的协程。
	maxAutoWorkers = 64

	// defaultScanStart / defaultScanEnd 是扫描时的默认端口区间。
	defaultScanStart = 1
	defaultScanEnd   = 65535
)

// sockStream / sockDgram 是 gopsutil 在 ConnectionStat.Type 中回填的
// socket 类型值。Windows 与 Linux 的平台实现都直接取 syscall 常量，
// 这里别名一份，避免在业务代码里散落魔法数字。
const (
	sockStream = syscall.SOCK_STREAM
	sockDgram  = syscall.SOCK_DGRAM
)

// ErrProcessNotAccessible 表示端口已被占用，但当前权限无法读取其归属进程
// （典型场景：Windows 非管理员、Linux 非 root）。调用方应据此做优雅降级提示。
var ErrProcessNotAccessible = errors.New("端口已被占用，但当前权限无法读取其归属进程信息（请尝试以管理员/root 身份运行）")

// PortInfo 是单个端口的体检结果，也是 core 包对 CLI 层与 MCP 层暴露的唯一数据契约。
type PortInfo struct {
	Port     int     `json:"port"`                   // 端口号
	Protocol string  `json:"protocol"`               // 协议：tcp / udp
	IsOpen   bool    `json:"is_open"`                // 是否处于监听/占用状态
	PID      int32   `json:"pid,omitempty"`          // 占用进程 PID，0 表示未获取到
	Name     string  `json:"name,omitempty"`         // 进程名
	Cmdline  string  `json:"cmdline,omitempty"`      // 进程启动命令行
	RSS      uint64  `json:"rss_bytes,omitempty"`    // 常驻内存 RSS（字节）
	CPU      float64 `json:"cpu_percent,omitempty"`  // CPU 占用百分比
	Exe      string  `json:"exe,omitempty"`          // 可执行文件完整路径
	Created  int64   `json:"created_unix,omitempty"` // 进程启动时间（Unix 秒）

	// Degraded 为 true 表示端口已被占用、但进程详情因权限不足而缺失，
	// 上层可据此打印「优雅降级」提示而不是报错。
	Degraded bool `json:"degraded,omitempty"`
	// Err 记录探测过程中的非致命错误（如权限不足），不影响 IsOpen 的结论。
	Err string `json:"error,omitempty"`
}

// ============================ 底层探测原语 ============================

// checkTCPOpen 通过一次极短的 TCP 握手判断端口是否有进程在监听。
// 采用「能否建立连接」而不是「是否被拒绝」来判断，语义更直观且跨平台一致。
func checkTCPOpen(port int, timeout time.Duration) bool {
	// 依次尝试 IPv4 与 IPv6 回环地址：仅监听 ::1 的服务也能被正确识别。
	targets := []string{
		fmt.Sprintf("127.0.0.1:%d", port),
		fmt.Sprintf("[::1]:%d", port),
	}
	for _, target := range targets {
		conn, err := net.DialTimeout("tcp", target, timeout)
		if err == nil {
			_ = conn.Close()
			return true
		}
	}
	return false
}

// connKey 是「端口 + 协议类型」的复合键。
// 同一个端口号可以同时被 TCP 与 UDP 占用，因此不能只用端口号做键。
type connKey struct {
	port    uint32
	sockTyp uint32
}

// buildProcIndex 扫描一次系统连接表，构建「端口 -> PID」的反向索引。
//
// 这是本包的性能关键点：gopsutil 的 ConnectionsPid 在 Windows 上并未实现
// （直接返回 ErrNotImplementedError），且逐个 PID 查询会造成 N 倍系统调用。
// 因此全量扫描只做一次，随后由所有并发协程共享该只读索引。
func buildProcIndex(ctx context.Context) (map[connKey]int32, error) {
	// "inet" 等价于 tcp4 + tcp6 + udp4 + udp6，一次拿全。
	conns, err := gnet.ConnectionsWithContext(ctx, "inet")
	if err != nil {
		return nil, err
	}
	index := make(map[connKey]int32, len(conns))
	for _, c := range conns {
		// 仅索引本地监听侧地址；PID 为 0 表示系统未能给出归属进程。
		if c.Pid <= 0 || c.Laddr.Port == 0 {
			continue
		}
		index[connKey{port: c.Laddr.Port, sockTyp: c.Type}] = c.Pid
	}
	return index, nil
}

// sockTypeOf 把对外的协议字符串映射为 gopsutil 使用的 socket 类型值。
// Windows 与 Linux 下 ConnectionStat.Type 均取自 syscall.SOCK_STREAM/SOCK_DGRAM。
func sockTypeOf(protocol string) uint32 {
	if normalizeProtocol(protocol) == ProtocolUDP {
		return sockDgram
	}
	return sockStream
}

// normalizeProtocol 归一化协议字符串，非法值一律回退为 tcp。
func normalizeProtocol(protocol string) string {
	if len(protocol) >= 3 {
		switch {
		case protocol[0] == 'u' || protocol[0] == 'U':
			return ProtocolUDP
		}
	}
	return ProtocolTCP
}

// ============================ 对外核心能力 ============================

// InspectPort 体检单个端口：先判断占用状态，再补齐归属进程的详情。
//
// 参数：
//   - port：端口号，取值 1-65535。
//   - protocol：ProtocolTCP 或 ProtocolUDP。
//
// 无论端口是否被占用都会返回一个非 nil 的 *PortInfo；
// 仅当端口号本身非法时才返回 error。
func InspectPort(port int, protocol string) (*PortInfo, error) {
	return inspectPort(context.Background(), port, protocol, DefaultDialTimeout, nil)
}

// inspectPort 是 InspectPort 的内部实现，额外支持复用已建好的进程索引。
// index 为 nil 时（例如单端口体检）按需即时构建一次。
func inspectPort(ctx context.Context, port int, protocol string, timeout time.Duration, index map[connKey]int32) (*PortInfo, error) {
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("非法端口号 %d：有效范围为 1-65535", port)
	}

	proto := normalizeProtocol(protocol)
	info := &PortInfo{Port: port, Protocol: proto}

	// 第一步：判断端口占用状态。UDP 无连接语义，不能靠 dial 判断，
	// 因此 UDP 仅依赖系统连接表；TCP 则以一次真实握手为准。
	if proto == ProtocolTCP {
		info.IsOpen = checkTCPOpen(port, timeout)
	}

	// 第二步：查询占用该端口的进程。
	if index == nil {
		built, err := buildProcIndex(ctx)
		if err != nil {
			// 拿不到系统连接表属非致命错误（常见于权限受限），
			// 记录原因后仍然返回端口占用状态，做到优雅降级。
			info.Degraded = true
			info.Err = fmt.Sprintf("读取系统连接表失败：%v", err)
			return info, nil
		}
		index = built
	}

	pid, found := index[connKey{port: uint32(port), sockTyp: sockTypeOf(proto)}]
	if found {
		info.IsOpen = true // 系统连接表能查到记录，说明端口确实被占用
		info.PID = pid
		fillProcessDetail(ctx, info)
		return info, nil
	}

	// 端口有监听但系统表查不到归属进程：标记降级，由上层提示提权。
	if info.IsOpen {
		info.Degraded = true
		info.Err = ErrProcessNotAccessible.Error()
	}
	return info, nil
}

// fillProcessDetail 补齐进程名、内存、CPU、命令行等体检指标。
//
// 单个字段获取失败不影响其他字段，但会被汇总进 Degraded/Err：
// 在 Windows 非管理员场景下，svchost.exe 这类 SYSTEM 进程可以成功打开，
// 却在读取 Name/Cmdline 时返回 "Access is denied"。此时必须如实标记降级，
// 否则上层只会看到一个空进程名，却无从得知原因。
func fillProcessDetail(ctx context.Context, info *PortInfo) {
	proc, err := process.NewProcessWithContext(ctx, info.PID)
	if err != nil {
		info.Degraded = true
		info.Err = ErrProcessNotAccessible.Error()
		return
	}

	// 记录首个字段级错误，用于向用户解释为何部分详情缺失。
	var firstErr error
	record := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	if name, err := proc.Name(); err == nil {
		info.Name = name
	} else {
		record(err)
	}
	if cmdline, err := proc.Cmdline(); err == nil {
		info.Cmdline = cmdline
	}
	if exe, err := proc.Exe(); err == nil {
		info.Exe = exe
	}
	if mem, err := proc.MemoryInfo(); err == nil && mem != nil {
		info.RSS = mem.RSS
	}
	if cpu, err := proc.CPUPercentWithContext(ctx); err == nil {
		info.CPU = cpu
	}
	if created, err := proc.CreateTimeWithContext(ctx); err == nil {
		info.Created = created / 1000 // gopsutil 返回毫秒，统一转为 Unix 秒
	}

	// 连进程名都读不到，说明进程详情整体不可用，如实标记为降级。
	if info.Name == "" && firstErr != nil {
		info.Degraded = true
		info.Err = fmt.Sprintf("%s（%v）", ErrProcessNotAccessible.Error(), firstErr)
	}
}

// ScanOptions 控制并发扫描的行为。零值即为一套可直接使用的默认配置。
type ScanOptions struct {
	Protocol string        // 协议，默认 tcp
	Workers  int           // 并发协程数，<=0 时按 CPU 核数自动推导
	Timeout  time.Duration // 单端口探测超时，<=0 时取 DefaultDialTimeout
}

// ScanPortsConcurrent 并发扫描 [start, end] 闭区间内的端口。
//
// 并发模型（本项目的核心亮点）：
//
//	                    ┌── worker 1 ──┐
//	ports channel  ────▶├── worker 2 ──┤────▶ results channel ────▶ 收集 + 排序
//	   (任务分发)       └── worker N ──┘        (结果汇聚)
//	                          ▲
//	                   sync.WaitGroup 负责等待全部 worker 退出后关闭 results，
//	                   从而让主协程的 for-range 能够安全结束。
//
// 设计要点：进程反向索引只构建一次并由所有 worker 只读共享，
// 避免 N 倍系统调用；结果按端口号升序排序，保证输出稳定可测。
func ScanPortsConcurrent(ctx context.Context, start, end int, opts ScanOptions) ([]PortInfo, error) {
	if start < 1 {
		start = defaultScanStart
	}
	if end > 65535 {
		end = defaultScanEnd
	}
	if start > end {
		return nil, fmt.Errorf("非法端口区间 %d-%d：起始端口不能大于结束端口", start, end)
	}

	protocol := normalizeProtocol(opts.Protocol)
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultDialTimeout
	}
	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.NumCPU() * 2
		if workers > maxAutoWorkers {
			workers = maxAutoWorkers
		}
	}
	total := end - start + 1
	if workers > total {
		workers = total
	}

	// 进程反向索引构建失败不应中断扫描，仅降级为「无进程详情」。
	index, _ := buildProcIndex(ctx)

	var wg sync.WaitGroup
	ports := make(chan int, workers)
	results := make(chan PortInfo, workers)

	// 任务生产者：独立协程投递端口，投递完毕即关闭任务通道。
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(ports)
		for port := start; port <= end; port++ {
			select {
			case <-ctx.Done():
				return // 上游取消，提前收工
			case ports <- port:
			}
		}
	}()

	// 任务消费者：启动固定数量的 worker 争抢端口任务。
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for port := range ports {
				info, err := inspectPort(ctx, port, protocol, timeout, index)
				if err != nil {
					continue // 区间已校验，理论上不会触发
				}
				select {
				case <-ctx.Done():
					return
				case results <- *info:
				}
			}
		}()
	}

	// 汇合协程：等全部 worker 退出后关闭结果通道，主协程的 range 才能结束。
	go func() {
		wg.Wait()
		close(results)
	}()

	// 主协程收集结果；只保留真正被占用的端口。
	// 结果通道容量为 workers，无需额外缓冲即可容纳 worker 的并发写入。
	occupied := make([]PortInfo, 0, 16)
	for info := range results {
		if info.IsOpen {
			occupied = append(occupied, info)
		}
	}

	// 并发收集的结果顺序不确定，统一按端口号升序排序，保证幂等与可测。
	sort.Slice(occupied, func(i, j int) bool {
		if occupied[i].Port == occupied[j].Port {
			return occupied[i].Protocol < occupied[j].Protocol
		}
		return occupied[i].Port < occupied[j].Port
	})

	if err := ctx.Err(); err != nil {
		return occupied, fmt.Errorf("扫描被取消：%w", err)
	}
	return occupied, nil
}

// ListOptions 控制「读取系统连接表」模式的行为。零值即为一套可直接使用的默认配置。
type ListOptions struct {
	Protocol string // 协议，默认 tcp
	All      bool   // false（默认）仅列出监听端口；true 则包含全部连接（含出站连接）
}

// ListOccupiedPorts 直接读取一次系统连接表，列出端口占用情况。
//
// 与 ScanPortsConcurrent 的关系：
//   - 前者读一次系统连接表即可拿到全部结果，耗时通常在毫秒级；
//   - 后者逐个端口做真实探测，用于覆盖连接表未登记的监听或收敛到指定区间。
//
// 两者返回的 *PortInfo 结构完全一致，上层可无差别渲染。
//
// 关于「只保留监听」：系统连接表同时包含大量**出站连接**（ESTABLISHED /
// TIME_WAIT / CLOSE_WAIT），它们占用的是客户端随机临时端口，并非「谁占了我的端口」。
// 默认只保留监听项（TCP 状态为 LISTEN；UDP 无连接状态，Status 为空），
// 以与 ScanPortsConcurrent（天然只能发现监听端口）保持语义一致；
// 传入 All=true 可拿到全部连接，用于排查出站连接归属。
func ListOccupiedPorts(ctx context.Context, opts ListOptions) ([]PortInfo, error) {
	proto := normalizeProtocol(opts.Protocol)
	wantSockType := sockTypeOf(proto)

	conns, err := gnet.ConnectionsWithContext(ctx, "inet")
	if err != nil {
		return nil, fmt.Errorf("读取系统连接表失败：%w", err)
	}

	// 边扫描边去重：同一端口可能有多条记录（如同时存在 IPv4/IPv6），
	// 已记录为「监听」的端口不再被出站连接覆盖。
	seen := make(map[uint32]int32, len(conns))
	listening := make(map[uint32]bool, len(conns))
	for _, c := range conns {
		if c.Laddr.Port == 0 || c.Type != wantSockType {
			continue
		}
		isListen := isListening(c)
		if !opts.All && !isListen {
			continue
		}
		if listening[c.Laddr.Port] && !isListen {
			continue // 已记录为监听，无需被出站连接覆盖
		}
		seen[c.Laddr.Port] = c.Pid
		listening[c.Laddr.Port] = isListen
	}

	infos := make([]PortInfo, 0, len(seen))
	for port, pid := range seen {
		info := PortInfo{
			Port:     int(port),
			Protocol: proto,
			IsOpen:   true,
			PID:      pid,
		}
		if pid > 0 {
			fillProcessDetail(ctx, &info)
		} else {
			info.Degraded = true
			info.Err = ErrProcessNotAccessible.Error()
		}
		infos = append(infos, info)
	}

	// 与并发扫描保持一致的排序，保证输出稳定可测。
	sort.Slice(infos, func(i, j int) bool { return infos[i].Port < infos[j].Port })
	return infos, nil
}

// isListening 判断一条连接记录是否代表「本机正在监听该端口」。
//
// TCP 会给出明确状态（LISTEN）；UDP 无连接语义，gopsutil 返回空状态，
// 因此空状态在 UDP 场景下即视为监听。
func isListening(c gnet.ConnectionStat) bool {
	switch c.Status {
	case "LISTEN":
		return true
	case "":
		// 仅 UDP 允许空状态，避免把异常记录误判为监听。
		return c.Type == sockDgram
	default:
		return false
	}
}

// KillPortProcess 结束占用指定端口的进程。
//
// force 为 true 时使用 Kill（强制终止），否则使用 Terminate（请求优雅退出）。
// 返回被结束进程的 PID，便于上层输出确认信息。
func KillPortProcess(port int, force bool) (int32, error) {
	info, err := InspectPort(port, ProtocolTCP)
	if err != nil {
		return 0, err
	}
	if !info.IsOpen {
		return 0, fmt.Errorf("端口 %d 当前未被占用，无需释放", port)
	}
	if info.PID <= 0 {
		return 0, fmt.Errorf("端口 %d 已被占用，但未能获取其 PID：%w", port, ErrProcessNotAccessible)
	}

	proc, err := process.NewProcess(info.PID)
	if err != nil {
		return 0, fmt.Errorf("无法打开进程 %d：%w", info.PID, err)
	}

	if force {
		err = proc.Kill()
	} else {
		err = proc.Terminate()
	}
	if err != nil {
		return info.PID, fmt.Errorf("结束进程 %d 失败（可能需要管理员/root 权限）：%w", info.PID, err)
	}
	return info.PID, nil
}
