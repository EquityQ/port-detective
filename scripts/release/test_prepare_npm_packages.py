import io
import json
import os
import stat
import sys
import tarfile
import tempfile
import unittest
import zipfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts" / "release"))

from prepare_npm_packages import TARGETS, package_name, prepare_packages


class PrepareNpmPackagesTests(unittest.TestCase):
    def test_stages_all_platform_packages_and_exact_optional_versions(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            dist = root / "dist"
            main = root / "npm" / "port-detective"
            output = root / "output"
            dist.mkdir()
            main.mkdir(parents=True)
            (main / "package.json").write_text(
                json.dumps({"name": "port-detective", "version": "1.1.0-dev"}),
                encoding="utf-8",
            )
            (main / "index.js").write_text("module.exports = {};\n", encoding="utf-8")
            (root / "LICENSE").write_text("MIT\n", encoding="utf-8")

            for platform, arch, goos, goarch, binary_name in TARGETS:
                archive = dist / f"port-detective_1.1.0_{goos}_{goarch}"
                payload = f"binary-{goos}-{goarch}".encode("ascii")
                if goos == "windows":
                    archive = Path(f"{archive}.zip")
                    with zipfile.ZipFile(archive, "w") as source:
                        source.writestr(f"port-detective_1.1.0/{binary_name}", payload)
                else:
                    archive = Path(f"{archive}.tar.gz")
                    with tarfile.open(archive, "w:gz") as source:
                        member = tarfile.TarInfo(f"port-detective_1.1.0/{binary_name}")
                        member.size = len(payload)
                        source.addfile(member, io.BytesIO(payload))

            names = prepare_packages("1.1.0", dist, main, output)
            self.assertEqual(len(names), 7)
            main_metadata = json.loads(
                (output / "port-detective" / "package.json").read_text(encoding="utf-8")
            )
            self.assertEqual(main_metadata["version"], "1.1.0")
            self.assertEqual(
                set(main_metadata["optionalDependencies"]),
                {package_name(platform, arch) for platform, arch, *_ in TARGETS},
            )
            self.assertTrue(
                all(version == "1.1.0" for version in main_metadata["optionalDependencies"].values())
            )

            for platform, arch, goos, goarch, binary_name in TARGETS:
                package_dir = output / package_name(platform, arch)
                metadata = json.loads((package_dir / "package.json").read_text(encoding="utf-8"))
                self.assertEqual(metadata["os"], [platform])
                self.assertEqual(metadata["cpu"], [arch])
                self.assertEqual(
                    (package_dir / "bin" / binary_name).read_bytes(),
                    f"binary-{goos}-{goarch}".encode("ascii"),
                )
                if goos != "windows" and os.name != "nt":
                    self.assertTrue(
                        (package_dir / "bin" / binary_name).stat().st_mode & stat.S_IXUSR
                    )


if __name__ == "__main__":
    unittest.main()
