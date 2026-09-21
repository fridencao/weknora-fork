# -*- coding: utf-8 -*-
"""StarKB 解析引擎（docreader 侧，Python 优先架构）。

链路：文件字节 → starkb-api `/parse/jobs`（编排 MinerU + 契约归一层）→
轮询至完成 → 取回契约包 document.md → Document(content=markdown)。

产出即 StarKB 解析契约（区块锚点 Markdown）：检索与溯源的坐标系由此固定。
环境变量：
  STARKB_API_URL   starkb-api 服务地址（默认 http://starkb-api:8300）
"""

from __future__ import annotations

import json
import logging
import os
import tempfile
import time
import urllib.request
import uuid

from docreader.models.document import Document
from docreader.parser.base_parser import BaseParser

logger = logging.getLogger(__name__)
logger.setLevel(logging.INFO)

STARKB_API_URL = os.environ.get("STARKB_API_URL", "http://starkb-api:8300")
POLL_INTERVAL = 3.0
POLL_TIMEOUT = 900.0


def _api(path: str, method: str = "GET", body: dict | None = None, timeout: float = 120.0):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(
        STARKB_API_URL.rstrip("/") + path, data=data, method=method,
        headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=timeout) as r:
        return json.loads(r.read())


class StarkbParser(BaseParser):
    """调用 starkb-api 的全管线解析器（MinerU 档位路由 + 契约归一层）。"""

    def parse_into_text(self, content: bytes) -> Document:
        # 1) 落临时文件并提交任务（form-data 传原始文件名以保留扩展名）
        tmp = tempfile.NamedTemporaryFile(
            prefix="starkb-", suffix="." + (self.file_type or "bin"), delete=False)
        try:
            tmp.write(content)
            tmp.close()
            boundary = "----starkbdocreader"
            fname = os.path.basename(self.file_name) or "upload.bin"
            body = (
                f"--{boundary}\r\n"
                f'Content-Disposition: form-data; name="file"; filename="{fname}"\r\n'
                f"Content-Type: application/octet-stream\r\n\r\n"
            ).encode() + content + f"\r\n--{boundary}--\r\n".encode()
            req = urllib.request.Request(
                STARKB_API_URL.rstrip("/") + "/parse/jobs/upload",
                data=body, method="POST",
                headers={"Content-Type": f"multipart/form-data; boundary={boundary}"})
            with urllib.request.urlopen(req, timeout=120) as r:
                job = json.loads(r.read())
        finally:
            os.unlink(tmp.name)
        job_id = job["job_id"]
        logger.info("starkb parse job=%s tier=%s", job_id, job.get("tier"))

        # 2) 同步执行（docreader gRPC 调用本身是同步语义）
        run = _api(f"/parse/jobs/{job_id}/run", "POST")
        if run.get("status") != "completed":
            raise RuntimeError(f"starkb 解析失败: {run}")

        # 3) 取契约包 markdown
        md = _api(f"/parse/jobs/{job_id}/markdown")
        return Document(content=md.get("markdown", ""))
