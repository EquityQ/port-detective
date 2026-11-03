"""Tests for release artifact version checks and the strict platform manifest gate."""

from __future__ import annotations

import contextlib
import hashlib
import io
import re
import shutil
import subprocess
import tarfile
import tempfile
import unittest
import uuid
import zipfile
from pathlib import Path
from unittest.mock import patch

from verify_artifacts import (
    EXECUTABLE_NAME_BY_GOOS,
    RELEASE_TARGETS,
    expected_archives,
    main,
    verify_archives,
    verify_checksums,
    verify_cli,
    verify_python,
    verify_release_manifest,
)


VERSION = "1.2.0"
CHECKSUM_FILE_NAME = "checksums.txt"


def new_scratch_dir(case: unittest.TestCase) -> Path:
    """Create a writable scratch directory that the test case removes afterwards.

    ``tempfile.TemporaryDirectory`` creates its root with mode ``0o700``. On a
    locked-down Windows CI sandbox that mode yields a directory the test process
    cannot create files in, so the root here is created with the platform default
    mode and removed explicitly.
    """
    root = Path(tempfile.gettempdir()) / f"pd-release-{uuid.uuid4().hex}"
    root.mkdir()
    case.addCleanup(shutil.rmtree, root, ignore_errors=True)
    return root


def archive_entries_for(
    target, *, prefix: str | None, executable: bool = True
) -> list[tuple[str, bytes | None]]:
    """Members for one archive: the platform executable plus README.md and LICENSE."""
    names = ["README.md", "LICENSE"]
    if executable:
        names.insert(0, EXECUTABLE_NAME_BY_GOOS[target.goos])
    entries: list[tuple[str, bytes | None]] = []
    for name in names:
        entries.append((f"{prefix}/{name}" if prefix else name, f"content of {name}".encode()))
    return entries


def write_archive(path: Path, entries: list[tuple[str, bytes | None]]) -> None:
    """Write a tar.gz/zip archive from (member name, bytes or None-for-directory) pairs."""
    if path.name.endswith(".zip"):
        with zipfile.ZipFile(path, "w") as archive:
            for name, content in entries:
                if content is None:
                    archive.writestr(name.rstrip("/") + "/", b"")
                else:
                    archive.writestr(name, content)
        return
    with tarfile.open(path, "w:gz") as archive:
        for name, content in entries:
            member = tarfile.TarInfo(name)
            if content is None:
                member.type = tarfile.DIRTYPE
                archive.addfile(member)
            else:
                member.size = len(content)
                archive.addfile(member, io.BytesIO(content))


def write_checksums(path: Path, archives: list[Path]) -> None:
    lines = [
        f"{hashlib.sha256(archive.read_bytes()).hexdigest()}  {archive.name}"
        for archive in archives
    ]
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


class ReleaseManifestFixture(unittest.TestCase):
    """Builds a synthetic ``dist`` directory shaped like GoReleaser output."""

    def setUp(self) -> None:
        self.root = new_scratch_dir(self)
        self.dist = self.root / "dist"
        self.dist.mkdir()
        self.checksums = self.dist / CHECKSUM_FILE_NAME

    def build_dist(
        self,
        *,
        prefix: str | None = None,
        skipped: tuple[tuple[str, str], ...] = (),
        entries_for=None,
    ) -> list[Path]:
        """Create one archive per release target; ``skipped`` holds (goos, goarch) pairs to omit."""
        archives: list[Path] = []
        for target in RELEASE_TARGETS:
            if (target.goos, target.goarch) in skipped:
                continue
            archive = self.dist / f"port-detective_{VERSION}_{target.goos}_{target.goarch}{target.archive_suffix}"
            entries = (
                entries_for(target)
                if entries_for is not None
                else archive_entries_for(target, prefix=prefix)
            )
            write_archive(archive, entries)
            archives.append(archive)
        return archives

    def write_all_checksums(self, archives: list[Path]) -> None:
        write_checksums(self.checksums, archives)


class ReleaseManifestTests(ReleaseManifestFixture):
    def test_expected_archives_cover_exactly_the_goreleaser_matrix(self) -> None:
        self.assertEqual(
            set(expected_archives(VERSION)),
            {
                f"port-detective_{VERSION}_linux_amd64.tar.gz",
                f"port-detective_{VERSION}_linux_arm64.tar.gz",
                f"port-detective_{VERSION}_darwin_amd64.tar.gz",
                f"port-detective_{VERSION}_darwin_arm64.tar.gz",
                f"port-detective_{VERSION}_windows_amd64.zip",
                f"port-detective_{VERSION}_windows_arm64.zip",
            },
        )

    def test_accepts_complete_six_platform_manifest(self) -> None:
        archives = self.build_dist()
        self.write_all_checksums(archives)

        verify_release_manifest(self.dist, VERSION, self.checksums)

    def test_accepts_executable_under_a_single_directory_prefix(self) -> None:
        archives = self.build_dist(prefix=f"port-detective_{VERSION}")
        self.write_all_checksums(archives)

        verify_release_manifest(self.dist, VERSION, self.checksums)

    def test_rejects_missing_platform_archive(self) -> None:
        archives = self.build_dist(skipped=(("windows", "arm64"),))
        self.write_all_checksums(archives)

        with self.assertRaisesRegex(
            SystemExit,
            re.escape("missing platform archive(s): port-detective_1.2.0_windows_arm64.zip (windows/arm64)"),
        ):
            verify_release_manifest(self.dist, VERSION, self.checksums)

    def test_rejects_unexpected_extra_archive(self) -> None:
        archives = self.build_dist()
        extra = self.dist / f"port-detective_{VERSION}_freebsd_amd64.tar.gz"
        write_archive(
            extra,
            [("port-detective", b"binary"), ("README.md", b"readme"), ("LICENSE", b"license")],
        )
        self.write_all_checksums([*archives, extra])

        with self.assertRaisesRegex(
            SystemExit, re.escape("unexpected release archive(s): port-detective_1.2.0_freebsd_amd64.tar.gz")
        ):
            verify_release_manifest(self.dist, VERSION, self.checksums)

    def test_rejects_archive_with_no_checksum_entry(self) -> None:
        archives = self.build_dist()
        unchecksummed = self.dist / f"port-detective_{VERSION}_darwin_amd64.tar.gz"
        self.write_all_checksums([a for a in archives if a.name != unchecksummed.name])

        with self.assertRaisesRegex(
            SystemExit, re.escape("release archive(s) with no checksum entry: port-detective_1.2.0_darwin_amd64.tar.gz")
        ):
            verify_release_manifest(self.dist, VERSION, self.checksums)

    def test_rejects_checksum_entry_with_no_archive(self) -> None:
        archives = self.build_dist()
        self.write_all_checksums(archives)
        with self.checksums.open("a", encoding="utf-8") as handle:
            handle.write(f"{'0' * 64}  port-detective_{VERSION}_plan9_amd64.tar.gz\n")

        with self.assertRaisesRegex(
            SystemExit, re.escape("checksum entry with no matching release archive: port-detective_1.2.0_plan9_amd64.tar.gz")
        ):
            verify_release_manifest(self.dist, VERSION, self.checksums)

    def test_rejects_checksum_digest_mismatch(self) -> None:
        archives = self.build_dist()
        self.write_all_checksums(archives)
        lines = self.checksums.read_text(encoding="utf-8").splitlines()
        bad = f"{'0' * 64}  {archives[0].name}"
        self.checksums.write_text("\n".join([bad, *lines[1:]]) + "\n", encoding="utf-8")

        with self.assertRaisesRegex(SystemExit, "checksum mismatch for port-detective_"):
            verify_release_manifest(self.dist, VERSION, self.checksums)

    def test_rejects_missing_executable_inside_archive(self) -> None:
        def without_executable(target):
            return archive_entries_for(target, prefix=None, executable=False)

        archives = self.build_dist(entries_for=without_executable)
        self.write_all_checksums(archives)

        with self.assertRaisesRegex(
            SystemExit, re.escape("is missing the linux/amd64 executable 'port-detective'")
        ):
            verify_release_manifest(self.dist, VERSION, self.checksums)

    def test_rejects_windows_archive_without_exe_suffix(self) -> None:
        def unix_named(target):
            entries = archive_entries_for(target, prefix=None, executable=False)
            return [("port-detective", b"binary"), *entries]

        archives = self.build_dist(entries_for=unix_named)
        self.write_all_checksums(archives)

        with self.assertRaisesRegex(
            SystemExit, re.escape("is missing the windows/amd64 executable 'port-detective.exe'")
        ):
            verify_release_manifest(self.dist, VERSION, self.checksums)

    def test_rejects_directory_entry_named_like_the_executable(self) -> None:
        def directory_only(target):
            entries = archive_entries_for(target, prefix=None, executable=False)
            return [(EXECUTABLE_NAME_BY_GOOS[target.goos], None), *entries]

        archives = self.build_dist(entries_for=directory_only)
        self.write_all_checksums(archives)

        with self.assertRaisesRegex(
            SystemExit, re.escape("found a same-named directory instead of a regular file")
        ):
            verify_release_manifest(self.dist, VERSION, self.checksums)

    def test_rejects_executable_nested_below_one_directory_prefix(self) -> None:
        def too_deep(target):
            entries = archive_entries_for(target, prefix="outer/inner")
            return entries

        archives = self.build_dist(entries_for=too_deep)
        self.write_all_checksums(archives)

        with self.assertRaisesRegex(
            SystemExit, re.escape("is missing the linux/amd64 executable 'port-detective'")
        ):
            verify_release_manifest(self.dist, VERSION, self.checksums)

    def test_rejects_directory_entry_instead_of_readme(self) -> None:
        def readme_directory(target):
            entries = archive_entries_for(target, prefix=None, executable=True)
            return [
                (name, None) if name == "README.md" else (name, content)
                for name, content in entries
            ]

        archives = self.build_dist(entries_for=readme_directory)
        self.write_all_checksums(archives)

        with self.assertRaisesRegex(SystemExit, re.escape("is missing README.md")):
            verify_release_manifest(self.dist, VERSION, self.checksums)


class ReleaseManifestVersionTests(ReleaseManifestFixture):
    def test_rejects_manifest_for_a_different_version(self) -> None:
        archives = self.build_dist()
        self.write_all_checksums(archives)

        with self.assertRaisesRegex(SystemExit, "missing platform archive"):
            verify_release_manifest(self.dist, "1.2.1", self.checksums)

    def test_rejects_invalid_version(self) -> None:
        self.build_dist()
        with self.assertRaisesRegex(SystemExit, "invalid release version"):
            verify_release_manifest(self.dist, "../1.2.0", self.checksums)


class ReleaseManifestCommandTests(ReleaseManifestFixture):
    def run_main(self, arguments: list[str]) -> str:
        output = io.StringIO()
        with patch("sys.argv", ["verify_artifacts.py", *arguments]):
            with contextlib.redirect_stdout(output):
                self.assertEqual(main(), 0)
        return output.getvalue()

    def test_cli_reports_the_verified_platform_manifest(self) -> None:
        archives = self.build_dist()
        self.write_all_checksums(archives)

        stdout = self.run_main(
            [
                "--expected-version",
                VERSION,
                "--dist",
                str(self.dist),
                "--checksums",
                str(self.checksums),
                "--require-platform-manifest",
            ]
        )

        self.assertIn("verified the exact 6-platform release manifest", stdout)

    def test_requires_dist_and_checksums_for_the_strict_gate(self) -> None:
        strict = "--require-platform-manifest"
        for arguments, missing in (
            (["--expected-version", VERSION, strict, "--checksums", str(self.checksums)], "--dist"),
            (["--expected-version", VERSION, strict, "--dist", str(self.dist)], "--checksums"),
        ):
            with self.subTest(missing=missing):
                with patch("sys.argv", ["verify_artifacts.py", *arguments]):
                    with self.assertRaisesRegex(SystemExit, re.escape(f"requires {missing}")):
                        main()

    def test_non_strict_checks_remain_available(self) -> None:
        """The pre-existing checks keep working for call sites that do not opt in."""
        def documents_only(target):
            return archive_entries_for(target, prefix=None, executable=False)

        archives = self.build_dist(entries_for=documents_only)
        self.write_all_checksums(archives)

        verify_checksums(self.dist, self.checksums)
        verify_archives(self.dist)


class VerifyCliTests(unittest.TestCase):
    def test_resolves_relative_cli_path_before_execution(self) -> None:
        relative = Path("port-detective")
        result = subprocess.CompletedProcess(
            args=[str(relative), "--version"],
            returncode=0,
            stdout="port-detective 1.1.0\n",
            stderr="",
        )
        with patch("verify_artifacts.subprocess.run", return_value=result) as run:
            verify_cli(relative, "1.1.0")

        self.assertEqual(run.call_args.args[0], [str(relative.resolve()), "--version"])


class VerifyPythonTests(unittest.TestCase):
    def setUp(self) -> None:
        self.root = new_scratch_dir(self) / "python"
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
