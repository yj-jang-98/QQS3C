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
    # Selects which Go engine folder model.py builds and starts.
    # Use "encrypted" to run QQS3C/interface/controller/py/vempc/encrypted.
    # engine_variant: str = "unencrypted"
    engine_variant: str = "encrypted"
    

    # "variational" uses the sampling controller; "standard" uses the
    # deterministic condensed MPC solver.
    backend: str = "variational"
    sample_time: float = 0.02
    horizon: int = 12

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

    q_diag: tuple[float, ...] = (1.0, 10.0, 0.1, 1.0)
    r_diag: tuple[float, ...] = (0.01,)
    qf_scale: float = 10.0

    # The VEMPC state/input bounds are deliberately expressed in the same units
    # used by the QQS3C controller loop.
    alpha_max: float = 0.35
    u_max: float = 1.0

    # Variational MPC tuning. These are the online sampling parameters that the
    # Go engine uses after the observer state is corrected from measurements.
    sigma0: float = 0.25
    lambda_param: float = 0.1
    k_samples: int = 150
    cheb_order: int = 3
    cheb_bound: float = 5.0
    cheb_eta: float = 500.0
    cheb_clip: bool = True

    # CKKS engine settings. These are consumed only by the encrypted Go engine;
    # the unencrypted engine ignores the extra JSON fields.
    encrypted_cache_steps: int = 40
    encrypted_workers: int = 4
    ckks_log_n: int = 13
    ckks_log_q: tuple[int, ...] = (33, 30, 30, 30)
    ckks_log_p: tuple[int, ...] = (35,)
    ckks_log_default_scale: int = 30

    # Plot-only metadata used by ctrl_vempc.py when it annotates the saved
    # result figure. These do not affect the Go engine.
    plot_phase_mode: str = "hardware_swing_up"
    state_feedback_steps: int = 100
    outside_controller_angle_deg: float = 20.0
    swing_up_display_steps: int = 25

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
            "encryptedCacheSteps": self.encrypted_cache_steps,
            "encryptedWorkers": self.encrypted_workers,
            "ckksLogN": self.ckks_log_n,
            "ckksLogQ": list(self.ckks_log_q),
            "ckksLogP": list(self.ckks_log_p),
            "ckksLogDefaultScale": self.ckks_log_default_scale,
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
