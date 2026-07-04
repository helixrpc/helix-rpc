import json
import os
import asyncio
import logging

class Config:
    def __init__(
        self,
        host: str = "127.0.0.1",
        port: int = 8080,
        disable_metrics: bool = False,
        disable_health: bool = False,
        disable_gzip: bool = False,
        disable_deadline: bool = False,
        rate_limit_rate: float = 100.0,
        rate_limit_burst: int = 10,
        enable_jwt_auth: bool = False,
        jwt_secret: str = "",
        enable_api_key_auth: bool = False,
        api_key: str = "",
    ):
        self.host = host
        self.port = port
        self.disable_metrics = disable_metrics
        self.disable_health = disable_health
        self.disable_gzip = disable_gzip
        self.disable_deadline = disable_deadline
        self.rate_limit_rate = rate_limit_rate
        self.rate_limit_burst = rate_limit_burst
        self.enable_jwt_auth = enable_jwt_auth
        self.jwt_secret = jwt_secret
        self.enable_api_key_auth = enable_api_key_auth
        self.api_key = api_key

def load_config(path: str) -> Config:
    with open(path, "r") as f:
        data = json.load(f)
    return Config(**data)

def watch_config(path: str, on_change) -> None:
    async def _watch():
        try:
            last_mod = os.path.getmtime(path)
        except OSError:
            last_mod = 0
            
        while True:
            await asyncio.sleep(2.0)
            try:
                modified = os.path.getmtime(path)
                if modified > last_mod:
                    last_mod = modified
                    cfg = load_config(path)
                    logging.info(f"🧬 [Helix] Dynamic config reload from {path} succeeded.")
                    on_change(cfg)
            except Exception as e:
                logging.warning(f"✗ [Helix] Failed to reload config: {e}")
                
    asyncio.create_task(_watch())
