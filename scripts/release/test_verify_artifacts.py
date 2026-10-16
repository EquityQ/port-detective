"""Tests for release artifact version checks."""

from __future__ import annotations

import io
import tarfile
import tempfile
import unittest
import zipfile
from pathlib import Path

from verify_artifacts import verify_python


class VerifyPythonTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp_dir = tempfile.TemporaryDirectory()
        self.root = Path(self.temp_dir.name) / "python"
        self.dist = self.root / "dist"
        (self.root / "port_detective").mkdir(parents=True)
        self.dist.mkdir()
        (self.root / "pyproject.toml").write_text(
            '[project]\nversion = "1.1.0"\n', encoding="utf-8"
        )
        (self.root / "port_detective" / "__init__.py").write_text(
            '__version__ = "1.1.0"\n', encoding="utf-8"
        )
        self._write_wheel("1.1.0")

    def tearDown(self) -> None:
        self.temp_dir.cleanup()

    def _write_wheel(self, version: str) -> None:
        with zipfile.ZipFile(self.dist / "port_detective.whl", "w") as archive:
            archive.writestr(
                "port_detective-1.1.0.dist-info/METADATA",
                f"Metadata-Version: 2.1\nName: port-detective\nVersion: {version}\n",
            )

    def _write_sdist(self, version: str) -> None:
        path = self.dist / "port_detective.tar.gz"
        with tarfile.open(path, "w:gz") as archive:
            for name in (
                "port_detective-1.1.0/PKG-INFO",
                "port_detective-1.1.0/port_detective.egg-info/PKG-INFO",
            ):
                metadata = f"Metadata-Version: 2.1\nName: port-detective\nVersion: {version}\n".encode()
                member = tarfile.TarInfo(name)
                member.size = len(metadata)
                archive.addfile(member, io.BytesIO(metadata))

    def test_accepts_matching_wheel_and_sdist_versions(self) -> None:
        self._write_sdist("1.1.0")
        verify_python(self.root, self.dist, "1.1.0")

    def test_rejects_sdist_version_mismatch(self) -> None:
        self._write_sdist("1.0.9")
        with self.assertRaisesRegex(SystemExit, "sdist version is 1.0.9"):
            verify_python(self.root, self.dist, "1.1.0")


if __name__ == "__main__":
    unittest.main()
