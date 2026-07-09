import json
import os
import subprocess
from pathlib import Path

from config import DEFAULT_CONFIG, write_engine_config


class GoVEMPCController:
    def __init__(self, config=DEFAULT_CONFIG, rebuild=True):
        self.config = config
        self.base_dir = Path(__file__).resolve().parent
        self.engine_dir = self.base_dir / "unencrypted"
        self.binary_path = self.engine_dir / ("vempc_engine.exe" if os.name == "nt" else "vempc_engine")
        self.config_path = write_engine_config(config)
        self.process = None
        if rebuild or not self.binary_path.exists():
            self._build_engine()
        self._start_engine()

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc_value, traceback):
        self.close()

    def _build_engine(self):
        env = os.environ.copy()
        env["GOCACHE"] = str(self.engine_dir / ".gocache")
        cmd = ["go", "build", "-buildvcs=false", "-o", str(self.binary_path), "."]
        result = subprocess.run(
            cmd,
            cwd=self.engine_dir,
            capture_output=True,
            text=True,
            check=False,
            env=env,
        )
        if result.returncode != 0:
            raise RuntimeError(
                "failed to build Go VEMPC engine\n"
                f"stdout:\n{result.stdout}\n"
                f"stderr:\n{result.stderr}"
            )

    def _start_engine(self):
        cmd = [str(self.binary_path), "--config", str(self.config_path)]
        self.process = subprocess.Popen(
            cmd,
            cwd=self.engine_dir,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            bufsize=1,
        )

    def compute_control(self, y):
        if self.process is None or self.process.stdin is None or self.process.stdout is None:
            raise RuntimeError("Go VEMPC engine is not running")
        if self.process.poll() is not None:
            raise RuntimeError(f"Go VEMPC engine exited early\n{self._collect_stderr()}")

        request = {"type": "measure", "y": [float(y[0]), float(y[1])]}
        self.process.stdin.write(json.dumps(request) + "\n")
        self.process.stdin.flush()

        line = self.process.stdout.readline()
        if not line:
            raise RuntimeError(f"Go VEMPC engine closed stdout\n{self._collect_stderr()}")

        response = json.loads(line)
        if response.get("error"):
            raise RuntimeError(response["error"])
        return float(response["u"])

    def close(self):
        if self.process is None:
            return
        try:
            if self.process.poll() is None and self.process.stdin is not None:
                self.process.stdin.write(json.dumps({"type": "shutdown"}) + "\n")
                self.process.stdin.flush()
        except OSError:
            pass
        finally:
            try:
                self.process.wait(timeout=2)
            except subprocess.TimeoutExpired:
                self.process.kill()
            self.process = None

    def _collect_stderr(self):
        if self.process is None or self.process.stderr is None:
            return ""
        try:
            return self.process.stderr.read().strip()
        except OSError:
            return ""
