import os
import subprocess
from dataclasses import replace
from pathlib import Path

from config import DEFAULT_CONFIG, write_engine_config


def _engine_paths():
    base_dir = Path(__file__).resolve().parent
    engine_dir = base_dir / "encrypted"
    binary_path = engine_dir / ("vempc_engine.exe" if os.name == "nt" else "vempc_engine")
    return engine_dir, binary_path


def _needs_rebuild(engine_dir: Path, binary_path: Path) -> bool:
    if not binary_path.exists():
        return True
    binary_mtime = binary_path.stat().st_mtime
    for source_path in engine_dir.rglob("*.go"):
        if source_path.stat().st_mtime > binary_mtime:
            return True
    for dependency_path in (engine_dir / "go.mod", engine_dir / "go.sum"):
        if dependency_path.exists() and dependency_path.stat().st_mtime > binary_mtime:
            return True
    return False


def _build_engine(engine_dir: Path, binary_path: Path):
    env = os.environ.copy()
    env["GOCACHE"] = str(engine_dir / ".gocache")
    env.setdefault("GOSUMDB", "off")
    result = subprocess.run(
        ["go", "build", "-buildvcs=false", "-o", str(binary_path), "."],
        cwd=engine_dir,
        capture_output=True,
        text=True,
        check=False,
        env=env,
    )
    if result.returncode != 0:
        raise RuntimeError(
            "failed to build encrypted Go VEMPC engine\n"
            f"stdout:\n{result.stdout}\n"
            f"stderr:\n{result.stderr}"
        )


def run_offline(config=DEFAULT_CONFIG, rebuild=False):
    config = replace(config, engine_variant="encrypted")
    engine_dir, binary_path = _engine_paths()
    config_path = write_engine_config(config)

    if rebuild or _needs_rebuild(engine_dir, binary_path):
        _build_engine(engine_dir, binary_path)

    result = subprocess.run(
        [str(binary_path), "--config", str(config_path), "--offline-only"],
        cwd=engine_dir,
        check=False,
    )
    if result.returncode != 0:
        raise RuntimeError("failed to generate encrypted offline cache")

    cache_dir = engine_dir / "runtime" / "ckks_cache"
    print(f"encrypted offline cache ready: {cache_dir}")


def main():
    run_offline()


if __name__ == "__main__":
    main()
