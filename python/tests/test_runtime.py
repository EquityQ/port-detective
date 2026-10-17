"""Tests for the verified lazy CLI downloader."""

import hashlib
import io
import os
import tarfile
import tempfile
import unittest
import zipfile
from pathlib import Path
from unittest.mock import patch

from port_detective.runtime import (
    RuntimeDownloadError,
    _checksum,
    _extract_binary,
    _safe_member,
    auto_download_enabled,
    download_cli,
    platform_target,
)


class TestRuntimeDownload(unittest.TestCase):
    def _zip_asset(self, name: str, content: bytes) -> bytes:
        output = io.BytesIO()
        with zipfile.ZipFile(output, "w", zipfile.ZIP_DEFLATED) as archive:
            archive.writestr(f"port-detective/{name}", content)
        return output.getvalue()

    def test_downloads_verifies_extracts_and_reuses_cached_binary(self):
        binary = b"fake-windows-cli"
        archive_name = "port-detective_1.1.0_windows_amd64.zip"
        archive = self._zip_asset("port-detective.exe", binary)
        checksums = f"{hashlib.sha256(archive).hexdigest()}  {archive_name}\n".encode()

        with tempfile.TemporaryDirectory() as directory:
            calls = []

            def fake_download(url):
                calls.append(url)
                return archive if url.endswith(archive_name) else checksums

            with patch("port_detective.runtime.platform_target", return_value=("windows", "amd64", "port-detective.exe")), \
                 patch("port_detective.runtime._release_version", return_value="1.1.0"), \
                 patch("port_detective.runtime._download", side_effect=fake_download):
                path = Path(download_cli(destination=Path(directory)))
                self.assertEqual(path.read_bytes(), binary)
                self.assertEqual(Path(download_cli(destination=Path(directory))), path)
                path.write_bytes(b"tampered")
                self.assertEqual(Path(download_cli(destination=Path(directory))).read_bytes(), binary)

            self.assertEqual(len(calls), 4)

    def test_rejects_checksum_mismatch(self):
        archive_name = "port-detective_1.1.0_windows_amd64.zip"
        archive = self._zip_asset("port-detective.exe", b"binary")
        checksums = ("0" * 64 + "  " + archive_name + "\n").encode()
        with tempfile.TemporaryDirectory() as directory, \
             patch("port_detective.runtime.platform_target", return_value=("windows", "amd64", "port-detective.exe")), \
             patch("port_detective.runtime._release_version", return_value="1.1.0"), \
             patch("port_detective.runtime._download", side_effect=[archive, checksums]):
            with self.assertRaises(RuntimeDownloadError):
                download_cli(destination=Path(directory))
            self.assertEqual(list(Path(directory).iterdir()), [])

    def test_cache_is_scoped_to_release_version(self):
        binary = b"versioned-cli"
        archive_110 = self._zip_asset("port-detective.exe", binary + b"-110")
        archive_111 = self._zip_asset("port-detective.exe", binary + b"-111")
        assets = {
            "port-detective_1.1.0_windows_amd64.zip": archive_110,
            "port-detective_1.1.1_windows_amd64.zip": archive_111,
        }

        def fake_download(url):
            for name, content in assets.items():
                if url.endswith(name):
                    return content
            archive_name = next(name for name in assets if f"v{name.split('_')[1]}" in url)
            archive = assets[archive_name]
            return f"{hashlib.sha256(archive).hexdigest()}  {archive_name}\n".encode()

        with tempfile.TemporaryDirectory() as directory, \
             patch("port_detective.runtime.platform_target", return_value=("windows", "amd64", "port-detective.exe")), \
             patch("port_detective.runtime._download", side_effect=fake_download):
            first = Path(download_cli(version="1.1.0", destination=Path(directory)))
            second = Path(download_cli(version="1.1.1", destination=Path(directory)))
            self.assertNotEqual(first, second)
            self.assertEqual(first.parts[-3:], ("1.1.0", "windows-amd64", "port-detective.exe"))
            self.assertEqual(second.parts[-3:], ("1.1.1", "windows-amd64", "port-detective.exe"))
            self.assertNotEqual(first.read_bytes(), second.read_bytes())

    def test_rejects_unsupported_platform(self):
        with patch("port_detective.runtime.platform.system", return_value="FreeBSD"), \
             patch("port_detective.runtime.platform.machine", return_value="amd64"):
            with self.assertRaises(RuntimeDownloadError):
                platform_target()

    def test_platform_aliases_and_download_toggle(self):
        cases = [
            ("Windows", "AMD64", ("windows", "amd64", "port-detective.exe")),
            ("Darwin", "x86_64", ("darwin", "amd64", "port-detective")),
            ("Linux", "aarch64", ("linux", "arm64", "port-detective")),
        ]
        for system, machine, expected in cases:
            with self.subTest(system=system, machine=machine), \
                 patch("port_detective.runtime.platform.system", return_value=system), \
                 patch("port_detective.runtime.platform.machine", return_value=machine):
                self.assertEqual(platform_target(), expected)

        with patch.dict(os.environ, {"PORT_DETECTIVE_AUTO_DOWNLOAD": "0"}):
            self.assertFalse(auto_download_enabled())
        with patch.dict(os.environ, {"PORT_DETECTIVE_AUTO_DOWNLOAD": "yes"}):
            self.assertTrue(auto_download_enabled())

    def test_archive_helpers_cover_tar_and_reject_unsafe_data(self):
        binary = b"tar-cli"
        output = io.BytesIO()
        with tarfile.open(fileobj=output, mode="w:gz") as archive:
            info = tarfile.TarInfo("release/port-detective")
            info.size = len(binary)
            archive.addfile(info, io.BytesIO(binary))
        self.assertEqual(
            _extract_binary(output.getvalue(), "release.tar.gz", "port-detective"), binary
        )
        digest = "a" * 64
        self.assertEqual(_checksum(f"{digest}  release.zip\n".encode(), "release.zip"), digest)
        with self.assertRaises(RuntimeDownloadError):
            _checksum(b"bad  other.zip\n", "release.zip")
        with self.assertRaises(RuntimeDownloadError):
            _safe_member("../port-detective")


if __name__ == "__main__":
    unittest.main()
