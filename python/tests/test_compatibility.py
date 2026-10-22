"""Exercise CLI discovery against a real, controllable child executable."""

import asyncio
import contextlib
import io
import json
import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from port_detective import (
    PortDetectiveExecutionError, PortDetectiveVersionError, async_inspect_port,
    clean_project, find_binary, inspect_port,
)
from port_detective.__main__ import main
from port_detective.core import _VALIDATED_BINARIES


class FixtureMixin:
    @classmethod
    def setUpClass(cls):
        go = os.getenv("GO_BINARY") or shutil.which("go")
        if not go:
            raise unittest.SkipTest("Real executable lifecycle tests require the project's Go toolchain")
        cls.directory = tempfile.TemporaryDirectory()
        cls.binary = Path(cls.directory.name) / ("port-detective.exe" if os.name == "nt" else "port-detective")
        source = Path(__file__).resolve().parents[2] / "npm/port-detective/test/fixtures/cli.go"
        subprocess.run([go, "build", "-o", str(cls.binary), str(source)], check=True, capture_output=True)

    @classmethod
    def tearDownClass(cls):
        cls.directory.cleanup()

    def setUp(self):
        _VALIDATED_BINARIES.clear()
        self.environment = patch.dict(os.environ, {
            "PORT_DETECTIVE_BIN": str(self.binary), "SDK_TEST_VERSION": "1.2.4",
            "SDK_TEST_VERSION_FAIL": "", "SDK_TEST_VERSION_DELAY_MS": "0",
            "SDK_TEST_VERSION_LOG": "", "SDK_TEST_ARGUMENTS": "",
            "SDK_TEST_COMMAND_DELAY_MS": "0",
        })
        self.environment.start()
        self.addCleanup(self.environment.stop)


class TestCompatibility(FixtureMixin, unittest.TestCase):
    def test_compatible_patch_and_development_builds(self):
        for version in ["1.2.0", "1.2.4", "1.2.0-dev"]:
            with self.subTest(version=version):
                _VALIDATED_BINARIES.clear()
                os.environ["SDK_TEST_VERSION"] = version
                self.assertTrue(inspect_port(3000).is_open)

    def test_rejects_incompatible_and_unrecognizable_versions(self):
        for version in ["1.1.4", "1.3.0", "2.2.0", "dev", "not-a-version"]:
            with self.subTest(version=version):
                os.environ["SDK_TEST_VERSION"] = version
                with self.assertRaisesRegex(PortDetectiveVersionError, r"requires CLI 1\.2\.x"):
                    inspect_port(3000)

    def test_explicit_env_path_and_checkout_candidates_are_validated(self):
        os.environ["SDK_TEST_VERSION"] = "2.0.0"
        with self.assertRaises(PortDetectiveVersionError):
            find_binary(str(self.binary))
        with self.assertRaises(PortDetectiveVersionError):
            find_binary()
        os.environ.pop("PORT_DETECTIVE_BIN")
        with patch("port_detective.core.shutil.which", return_value=str(self.binary)):
            with self.assertRaises(PortDetectiveVersionError):
                find_binary()
        previous = Path.cwd()
        try:
            os.chdir(self.directory.name)
            with patch("port_detective.core.shutil.which", return_value=None):
                with self.assertRaises(PortDetectiveVersionError):
                    find_binary()
        finally:
            os.chdir(previous)

    def test_version_cache_invalidates_when_file_changes(self):
        log = Path(self.directory.name) / "versions.log"
        log.unlink(missing_ok=True)
        os.environ["SDK_TEST_VERSION_LOG"] = str(log)
        inspect_port(3000)
        inspect_port(3000)
        self.assertEqual(len(log.read_text().splitlines()), 1)
        metadata = self.binary.stat()
        os.utime(self.binary, ns=(metadata.st_atime_ns, metadata.st_mtime_ns + 1000000000))
        os.environ["SDK_TEST_VERSION"] = "2.0.0"
        with self.assertRaises(PortDetectiveVersionError):
            inspect_port(3000)
        self.assertEqual(len(log.read_text().splitlines()), 2)

    def test_version_failure_and_timeout_preserve_error_details(self):
        os.environ["SDK_TEST_VERSION_FAIL"] = "1"
        with self.assertRaises(PortDetectiveExecutionError) as failure:
            inspect_port(3000)
        self.assertEqual(failure.exception.returncode, 7)
        self.assertIn("version unavailable", failure.exception.stderr)
        os.environ["SDK_TEST_VERSION_FAIL"] = ""
        os.environ["SDK_TEST_VERSION_DELAY_MS"] = "1000"
        with self.assertRaisesRegex(PortDetectiveExecutionError, "version check timed out"):
            inspect_port(3000, timeout=0.05)

    def test_project_cleanup_passes_only_requested_port_selection(self):
        record = Path(self.directory.name) / "arguments.json"
        os.environ["SDK_TEST_ARGUMENTS"] = str(record)
        self.assertEqual(clean_project(path=self.directory.name, ports=[3010, 3011]), [])
        self.assertEqual(json.loads(record.read_text()), ["clean", "-y", "--dry-run", "--ports", "3010,3011", "--json"])

    def test_timeout_budget_is_shared_by_version_check_and_command(self):
        os.environ["SDK_TEST_VERSION_DELAY_MS"] = "300"
        os.environ["SDK_TEST_COMMAND_DELAY_MS"] = "600"
        with self.assertRaisesRegex(PortDetectiveExecutionError, "timed out"):
            inspect_port(3000, timeout=0.8)

    def test_nonfinite_timeouts_are_rejected_before_discovery(self):
        for timeout in [float("nan"), float("inf"), -float("inf")]:
            with self.assertRaises(ValueError):
                inspect_port(3000, timeout=timeout)

    def test_module_doctor_passes_through_cli_output(self):
        environment = os.environ.copy()
        environment["PYTHONPATH"] = str(Path(__file__).resolve().parents[1])
        import sys
        result = subprocess.run([sys.executable, "-m", "port_detective", "doctor", "--json"],
                                capture_output=True, text=True, env=environment, check=False)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout), {"healthy": True})
        self.assertIn(str(self.binary.resolve()), result.stderr)

    def test_module_install_reports_status_separately_from_path(self):
        output, errors = io.StringIO(), io.StringIO()
        with patch("port_detective.__main__.download_cli", return_value=str(self.binary)), \
             contextlib.redirect_stdout(output), contextlib.redirect_stderr(errors):
            self.assertEqual(main(["install"]), 0)
        self.assertEqual(output.getvalue().strip(), str(self.binary))
        self.assertIn("verifying SHA-256", errors.getvalue())


class TestAsyncCompatibility(FixtureMixin, unittest.IsolatedAsyncioTestCase):
    async def test_async_version_discovery_keeps_event_loop_responsive(self):
        os.environ["SDK_TEST_VERSION_DELAY_MS"] = "200"
        task = asyncio.create_task(async_inspect_port(3000))
        await asyncio.sleep(0.02)
        self.assertFalse(task.done())
        self.assertTrue((await task).is_open)

    async def test_async_timeout_and_cancellation_reap_version_child(self):
        os.environ["SDK_TEST_VERSION_DELAY_MS"] = "5000"
        log = Path(self.directory.name) / "async-versions.log"
        os.environ["SDK_TEST_VERSION_LOG"] = str(log)
        with self.assertRaisesRegex(PortDetectiveExecutionError, "timed out"):
            await async_inspect_port(3000, timeout=0.1)
        task = asyncio.create_task(async_inspect_port(3000, timeout=None))
        await asyncio.sleep(0.1)
        task.cancel()
        with self.assertRaises(asyncio.CancelledError):
            await task
        for pid in log.read_text().splitlines():
            # gopsutil is unnecessary here: process handles disappear after reaping.
            if os.name != "nt":
                with self.assertRaises(ProcessLookupError):
                    os.kill(int(pid), 0)
            else:
                import ctypes
                from ctypes import wintypes
                kernel = ctypes.WinDLL("kernel32", use_last_error=True)
                kernel.OpenProcess.restype = wintypes.HANDLE
                kernel.OpenProcess.argtypes = [wintypes.DWORD, wintypes.BOOL, wintypes.DWORD]
                handle = kernel.OpenProcess(0x1000, False, int(pid))
                if handle:
                    code = wintypes.DWORD()
                    kernel.GetExitCodeProcess.argtypes = [wintypes.HANDLE, ctypes.POINTER(wintypes.DWORD)]
                    kernel.CloseHandle.argtypes = [wintypes.HANDLE]
                    kernel.GetExitCodeProcess(handle, ctypes.byref(code))
                    kernel.CloseHandle(handle)
                    self.assertNotEqual(code.value, 259, f"version child {pid} is still active")

    async def test_async_rejects_incompatible_version_before_inspection(self):
        os.environ["SDK_TEST_VERSION"] = "2.0.0"
        with self.assertRaises(PortDetectiveVersionError):
            await async_inspect_port(3000)
