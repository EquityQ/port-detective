"""Stage the Node.js client and platform CLI packages from Go release archives."""

from __future__ import annotations

import argparse
import json
import re
import shutil
import stat
import tarfile
import zipfile
from pathlib import Path, PurePosixPath


TARGETS = (
    ("win32", "x64", "windows", "amd64", "port-detective.exe"),
    ("win32", "arm64", "windows", "arm64", "port-detective.exe"),
    ("darwin", "x64", "darwin", "amd64", "port-detective"),
    ("darwin", "arm64", "darwin", "arm64", "port-detective"),
    ("linux", "x64", "linux", "amd64", "port-detective"),
    ("linux", "arm64", "linux", "arm64", "port-detective"),
)


def package_name(platform: str, arch: str) -> str:
    return f"port-detective-cli-{platform}-{arch}"


def validate_version(version: str) -> None:
    if not re.fullmatch(r"\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?", version):
        raise SystemExit(f"invalid release version: {version}")


def find_archive(dist: Path, version: str, goos: str, goarch: str) -> Path:
    patterns = (
        f"port-detective_{version}_{goos}_{goarch}.zip",
        f"port-detective_{version}_{goos}_{goarch}.tar.gz",
    )
    for pattern in patterns:
        matches = sorted(dist.glob(pattern))
        if matches:
            return matches[0]
    raise SystemExit(f"release archive not found for {goos}/{goarch} in {dist}")


def safe_archive_member(name: str) -> PurePosixPath:
    path = PurePosixPath(name.replace("\\", "/"))
    if path.is_absolute() or ".." in path.parts:
        raise SystemExit(f"unsafe archive member path: {name}")
    return path


def extract_binary(archive: Path, binary_name: str) -> bytes:
    if archive.suffix == ".zip":
        with zipfile.ZipFile(archive) as source:
            for member in source.infolist():
                path = safe_archive_member(member.filename)
                if not member.is_dir() and path.name == binary_name:
                    return source.read(member)
    else:
        with tarfile.open(archive, "r:gz") as source:
            for member in source.getmembers():
                path = safe_archive_member(member.name)
                if member.isfile() and path.name == binary_name:
                    handle = source.extractfile(member)
                    if handle is not None:
                        return handle.read()
    raise SystemExit(f"{binary_name} not found in {archive}")


def write_json(path: Path, value: dict[str, object]) -> None:
    path.write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")


def write_platform_package(
    output: Path,
    version: str,
    platform: str,
    arch: str,
    goos: str,
    goarch: str,
    binary_name: str,
    binary: bytes,
    license_path: Path,
) -> str:
    name = package_name(platform, arch)
    package_dir = output / name
    bin_dir = package_dir / "bin"
    bin_dir.mkdir(parents=True, exist_ok=True)
    (bin_dir / binary_name).write_bytes(binary)
    if platform != "win32":
        mode = (bin_dir / binary_name).stat().st_mode
        (bin_dir / binary_name).chmod(mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
    shutil.copy2(license_path, package_dir / "LICENSE")
    (package_dir / "README.md").write_text(
        f"# {name}\n\nPlatform CLI binary for `port-detective` {version} ({goos}/{goarch}).\n",
        encoding="utf-8",
    )
    write_json(
        package_dir / "package.json",
        {
            "name": name,
            "version": version,
            "description": f"Platform CLI binary for port-detective ({goos}/{goarch}).",
            "license": "MIT",
            "files": ["bin", "README.md", "LICENSE"],
            "bin": {"port-detective": f"bin/{binary_name}"},
            "os": [platform],
            "cpu": [arch],
            "engines": {"node": ">=22"},
            "repository": {
                "type": "git",
                "url": "https://github.com/EquityQ/port-detective.git",
            },
        },
    )
    return name


def prepare_packages(version: str, dist: Path, main_root: Path, output: Path) -> list[str]:
    validate_version(version)
    if not dist.is_dir():
        raise SystemExit(f"release archive directory not found: {dist}")
    if not (main_root / "package.json").is_file():
        raise SystemExit(f"main npm package is missing: {main_root / 'package.json'}")
    license_path = main_root.parent.parent / "LICENSE"
    if not license_path.is_file():
        raise SystemExit(f"license file not found: {license_path}")
    if output.exists() and any(output.iterdir()):
        raise SystemExit(f"output directory must be empty: {output}")
    output.mkdir(parents=True, exist_ok=True)

    package_names: list[str] = []
    for platform, arch, goos, goarch, binary_name in TARGETS:
        archive = find_archive(dist, version, goos, goarch)
        binary = extract_binary(archive, binary_name)
        package_names.append(
            write_platform_package(
                output,
                version,
                platform,
                arch,
                goos,
                goarch,
                binary_name,
                binary,
                license_path,
            )
        )

    main_dir = output / "port-detective"
    shutil.copytree(main_root, main_dir)
    main_metadata_path = main_dir / "package.json"
    main_metadata = json.loads(main_metadata_path.read_text(encoding="utf-8"))
    main_metadata["version"] = version
    main_metadata["optionalDependencies"] = {
        name: version for name in package_names
    }
    write_json(main_metadata_path, main_metadata)
    return [*package_names, "port-detective"]


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True)
    parser.add_argument("--dist", type=Path, required=True)
    parser.add_argument("--main-root", type=Path, default=Path("npm/port-detective"))
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    names = prepare_packages(args.version, args.dist, args.main_root, args.output)
    print(f"prepared npm packages: {', '.join(names)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
