import json
import math
from dataclasses import dataclass
from pathlib import Path


BASE_DIR = Path(__file__).resolve().parent
RUNTIME_DIR = BASE_DIR / "runtime"
RESULTS_DIR = BASE_DIR / "results"
# This JSON file is the contract between Python and the Go engine.
ENGINE_CONFIG_PATH = RUNTIME_DIR / "controller_config.json"


@dataclass(frozen=True)
class QQS3CVempcConfig:
    # "variational" uses the sampling controller; "standard" uses the
    # deterministic condensed MPC solver.
    backend: str = "variational"
    sample_time: float = 0.02
    horizon: int = 15

    # QQS3C discrete model at ts=0.02 s. These matrices are copied directly
    # into the exported JSON so the Go engine follows the QQS3C plant model
    # rather than rebuilding a different model internally.
    A: tuple[tuple[float, ...], ...] = (
        (1.0, 0.030113645559215516, 0.019997367415355752, 0.000200064333471879),
        (0.0, 1.0527770743116616, -2.611852908027329e-06, 0.020350628771579043),
        (0.0, 3.0374476096695693, 0.9997354557097574, 0.030113645559215516),
        (0.0, 5.32351994268726, -0.0002634475575763798, 1.0527770743116616),
    )
    B: tuple[tuple[float, ...], ...] = (
        (0.011192961922808683,),
        (0.011104816785830465,),
        (1.1247631387867096,),
        (1.120100159763518,),
    )
    C: tuple[tuple[float, ...], ...] = (
        (1.0, 0.0, 0.0, 0.0),
        (0.0, 1.0, 0.0, 0.0),
    )
    L: tuple[tuple[float, ...], ...] = (
        (0.9707019478870462, 0.061851572345613934),
        (0.016849919276809382, 0.9477040198078667),
        (11.525427142231656, 3.9803740532970653),
        (0.47580910538569043, 13.750046647644771),
    )

    q_diag: tuple[float, ...] = (5000.0, 400.0, 1.0, 1.0)
    r_diag: tuple[float, ...] = (1.0,)
    qf_scale: float = 10.0

    # The VEMPC state/input bounds are deliberately expressed in the same units
    # used by the QQS3C controller loop.
    alpha_max: float = math.radians(15.0)
    u_max: float = 15.0

    # Variational MPC tuning. These are the online sampling parameters that the
    # Go engine uses after the observer state is corrected from measurements.
    sigma0: float = 6.0
    lambda_param: float = 0.75
    k_samples: int = 512
    cheb_order: int = 7
    cheb_bound: float = 15.0
    cheb_eta: float = 1.0
    cheb_clip: bool = True

    x0: tuple[float, ...] = (0.0, 0.0, 0.0, 0.0)

    def to_engine_dict(self) -> dict:
        # Keep this schema aligned with unencrypted/internal/engineconfig/config.go.
        return {
            "backend": self.backend,
            "dt": self.sample_time,
            "N": self.horizon,
            "A": [list(row) for row in self.A],
            "B": [list(row) for row in self.B],
            "C": [list(row) for row in self.C],
            "L": [list(row) for row in self.L],
            "QDiag": list(self.q_diag),
            "RDiag": list(self.r_diag),
            "QfScale": self.qf_scale,
            "alphaMax": self.alpha_max,
            "uMax": self.u_max,
            "sigma0": self.sigma0,
            "lambda": self.lambda_param,
            "K": self.k_samples,
            "chebOrder": self.cheb_order,
            "chebBound": self.cheb_bound,
            "chebEta": self.cheb_eta,
            "chebClip": self.cheb_clip,
            "x0": list(self.x0),
        }


DEFAULT_CONFIG = QQS3CVempcConfig()


def write_engine_config(config: QQS3CVempcConfig = DEFAULT_CONFIG, path: Path = ENGINE_CONFIG_PATH) -> Path:
    # The controller rewrites this file at startup so Go always sees the latest
    # Python-side configuration without any manual synchronization step.
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8") as fh:
        json.dump(config.to_engine_dict(), fh, indent=2)
    return path
