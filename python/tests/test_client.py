"""
Tests for port_detective Python client.
Supports running under both pytest and python -m unittest.
"""

import json
import os
import sys
import unittest
from unittest.mock import patch, MagicMock, AsyncMock

# Ensure parent directory is in sys.path
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))

from port_detective.models import PortDiagnostic, ProjectInfo, KillResult
from port_detective.core import (
    inspect_port,
    scan_ports,
    free_port,
    clean_project,
    async_inspect_port,
    async_scan_ports,
    async_free_port,
    async_clean_project,
    find_binary,
    PortDetectiveError,
    PortDetectiveNotFoundError,
    PortDetectiveExecutionError,
)


class TestModels(unittest.TestCase):
    def test_project_info_deserialization(self):
        data = {
            "name": "my-web-app",
            "framework": "Next.js",
            "path": "/workspace/my-web-app",
            "version": "14.2.0",
        }
        info = ProjectInfo.from_dict(data)
        self.assertIsNotNone(info)
        self.assertEqual(info.name, "my-web-app")
        self.assertEqual(info.framework, "Next.js")
        self.assertEqual(info.path, "/workspace/my-web-app")
        self.assertEqual(info.version, "14.2.0")

        self.assertIsNone(ProjectInfo.from_dict(None))
        self.assertIsNone(ProjectInfo.from_dict({}))

    def test_port_diagnostic_deserialization(self):
        data = {
            "port": 3000,
            "is_open": True,
            "protocol": "tcp",
            "pid": 4567,
            "process_name": "node",
            "cmdline": "node server.js",
            "memory_mb": 128.5,
            "cpu_percent": 2.4,
            "status": "occupied",
            "project": {
                "name": "next-portal",
                "framework": "Next.js",
                "path": "/app",
                "version": "1.0.0",
            },
        }
        diag = PortDiagnostic.from_dict(data)
        self.assertEqual(diag.port, 3000)
        self.assertTrue(diag.is_open)
        self.assertEqual(diag.pid, 4567)
        self.assertEqual(diag.process_name, "node")
        self.assertEqual(diag.memory_mb, 128.5)
        self.assertIsNotNone(diag.project)
        self.assertEqual(diag.project.framework, "Next.js")

        d = diag.to_dict()
        self.assertEqual(d["port"], 3000)
        self.assertEqual(d["project"]["framework"], "Next.js")

    def test_kill_result_deserialization(self):
        data = {
            "port": 8080,
            "dry_run": True,
            "message": "Dry-run preview success",
            "diagnostic": {
                "port": 8080,
                "is_open": True,
                "pid": 9999,
                "process_name": "python",
                "memory_mb": 45.0,
            },
        }
        res = KillResult.from_dict(data)
        self.assertEqual(res.port, 8080)
        self.assertTrue(res.dry_run)
        self.assertIn("preview", res.message)
        self.assertIsNotNone(res.diagnostic)
        self.assertEqual(res.diagnostic.pid, 9999)


class TestSyncClient(unittest.TestCase):
    @patch("port_detective.core.find_binary", return_value="/mock/port-detective")
    @patch("subprocess.run")
    def test_inspect_port(self, mock_run, mock_find):
        payload = {
            "port": 8000,
            "is_open": True,
            "protocol": "tcp",
            "pid": 1122,
            "process_name": "uvicorn",
            "memory_mb": 64.0,
            "cpu_percent": 1.1,
            "status": "occupied",
            "project": {
                "name": "fastapi-api",
                "framework": "FastAPI",
            },
        }
        mock_run.return_value = MagicMock(
            returncode=0,
            stdout=json.dumps(payload),
            stderr="",
        )

        diag = inspect_port(8000)
        self.assertEqual(diag.port, 8000)
        self.assertEqual(diag.process_name, "uvicorn")
        self.assertEqual(diag.project.framework, "FastAPI")

        mock_run.assert_called_once_with(
            ["/mock/port-detective", "check", "8000", "--json"],
            capture_output=True,
            text=True,
            encoding="utf-8",
            cwd=None,
            check=False,
        )

    @patch("port_detective.core.find_binary", return_value="/mock/port-detective")
    @patch("subprocess.run")
    def test_scan_ports(self, mock_run, mock_find):
        payload = [
            {"port": 3000, "is_open": True, "pid": 100, "process_name": "node"},
            {"port": 8080, "is_open": True, "pid": 200, "process_name": "go"},
        ]
        mock_run.return_value = MagicMock(
            returncode=0,
            stdout=json.dumps(payload),
            stderr="",
        )

        results = scan_ports([3000, 8080])
        self.assertEqual(len(results), 2)
        self.assertEqual(results[0].port, 3000)
        self.assertEqual(results[1].port, 8080)

        mock_run.assert_called_once_with(
            ["/mock/port-detective", "scan", "--ports", "3000,8080", "--json"],
            capture_output=True,
            text=True,
            encoding="utf-8",
            cwd=None,
            check=False,
        )

    @patch("port_detective.core.find_binary", return_value="/mock/port-detective")
    @patch("subprocess.run")
    def test_free_port(self, mock_run, mock_find):
        payload = {
            "port": 5173,
            "dry_run": False,
            "message": "Process terminated",
        }
        mock_run.return_value = MagicMock(
            returncode=0,
            stdout=json.dumps(payload),
            stderr="",
        )

        res = free_port(5173, dry_run=False)
        self.assertEqual(res.port, 5173)
        self.assertFalse(res.dry_run)

        mock_run.assert_called_once_with(
            ["/mock/port-detective", "kill", "5173", "-y", "--json"],
            capture_output=True,
            text=True,
            encoding="utf-8",
            cwd=None,
            check=False,
        )

    @patch("port_detective.core.find_binary", return_value="/mock/port-detective")
    @patch("subprocess.run")
    def test_clean_project(self, mock_run, mock_find):
        payload = [
            {"port": 3000, "is_open": True, "pid": 500, "process_name": "vite"},
        ]
        mock_run.return_value = MagicMock(
            returncode=0,
            stdout=json.dumps(payload),
            stderr="",
        )

        cleaned = clean_project(path="/workspace/app", dry_run=True)
        self.assertEqual(len(cleaned), 1)
        self.assertEqual(cleaned[0].port, 3000)

        mock_run.assert_called_once_with(
            ["/mock/port-detective", "clean", "-y", "--dry-run", "--json"],
            capture_output=True,
            text=True,
            encoding="utf-8",
            cwd="/workspace/app",
            check=False,
        )

    @patch("port_detective.core.find_binary", return_value="/mock/port-detective")
    @patch("subprocess.run")
    def test_execution_error(self, mock_run, mock_find):
        mock_run.return_value = MagicMock(
            returncode=1,
            stdout="",
            stderr="Permission denied",
        )

        with self.assertRaises(PortDetectiveExecutionError) as ctx:
            inspect_port(80)
        self.assertEqual(ctx.exception.returncode, 1)
        self.assertIn("Permission denied", ctx.exception.stderr)


class TestAsyncClient(unittest.IsolatedAsyncioTestCase):
    @patch("port_detective.core.find_binary", return_value="/mock/port-detective")
    @patch("asyncio.create_subprocess_exec")
    async def test_async_inspect_port(self, mock_exec, mock_find):
        payload = {
            "port": 9000,
            "is_open": True,
            "pid": 3333,
            "process_name": "php",
            "status": "occupied",
        }
        mock_proc = MagicMock()
        mock_proc.returncode = 0
        mock_proc.communicate = AsyncMock(return_value=(json.dumps(payload).encode("utf-8"), b""))
        mock_exec.return_value = mock_proc

        diag = await async_inspect_port(9000)
        self.assertEqual(diag.port, 9000)
        self.assertEqual(diag.pid, 3333)

    @patch("port_detective.core.find_binary", return_value="/mock/port-detective")
    @patch("asyncio.create_subprocess_exec")
    async def test_async_scan_ports(self, mock_exec, mock_find):
        payload = [{"port": 6379, "is_open": True, "pid": 444, "process_name": "redis-server"}]
        mock_proc = MagicMock()
        mock_proc.returncode = 0
        mock_proc.communicate = AsyncMock(return_value=(json.dumps(payload).encode("utf-8"), b""))
        mock_exec.return_value = mock_proc

        res = await async_scan_ports([6379])
        self.assertEqual(len(res), 1)
        self.assertEqual(res[0].port, 6379)

    @patch("port_detective.core.find_binary", return_value="/mock/port-detective")
    @patch("asyncio.create_subprocess_exec")
    async def test_async_clean_and_free(self, mock_exec, mock_find):
        mock_proc = MagicMock()
        mock_proc.returncode = 0
        mock_proc.communicate = AsyncMock(return_value=(b"[]", b""))
        mock_exec.return_value = mock_proc

        cleaned = await async_clean_project(dry_run=True)
        self.assertEqual(cleaned, [])

        free_payload = {"port": 8080, "dry_run": True, "message": "preview"}
        mock_proc.communicate = AsyncMock(return_value=(json.dumps(free_payload).encode("utf-8"), b""))
        freed = await async_free_port(8080, dry_run=True)
        self.assertEqual(freed.port, 8080)


if __name__ == "__main__":
    unittest.main()
