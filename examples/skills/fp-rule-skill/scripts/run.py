#!/usr/bin/env python3
"""fp-rule-skill — OJK Fit & Proper 法规转清单（Skill 1 参考实现）。

本脚本是 SKILL.md 契约的完整实现。它在 WeKnora sandbox 容器内运行时：
  1. 直连 WeKnora-postgres 重建法规全文（从 chunks 表）
  2. 按 Pasal / 罗马分节切片
  3. 调 GLM API 提取 ChecklistItem[]
  4. 后校验（R1-R12）
  5. 写入 ojk_checklist_items 表

环境变量：
  WEKNORA_PG_DSN   — PostgreSQL 连接串（由 requirements.json 声明，WeKnora 注入）
  OJK_TENANT_ID    — 租户 ID（由 WeKnora 注入或默认 10011）
  OJK_RUN_ID       — 本次运行 ID（由后端传入）
  GLM_API_KEY      — GLM API key（运行时从 models 表读取，不落盘）

用法：
  python3 scripts/run.py                    # 从 WeKnora PG 读切片，调 LLM，写入库
  python3 scripts/run.py --dry-run          # 只打印切片和 prompt，不调 LLM
  python3 scripts/run.py --regulation POJK  # 只处理指定法规

退出码：0 = 成功，1 = 失败（error 信息写入 stderr）
"""
from __future__ import annotations

import json
import os
import re
import sys
import time
import urllib.request
import urllib.error
from typing import Any

try:
    import psycopg2
except ImportError:
    print("psycopg2 not installed; install with: pip install psycopg2-binary", file=sys.stderr)
    sys.exit(1)

# ---------------------------------------------------------------- 配置

PG_DSN = os.environ.get("WEKNORA_PG_DSN", "")
TENANT_ID = int(os.environ.get("OJK_TENANT_ID", "10011"))
RUN_ID = os.environ.get("OJK_RUN_ID", "")
DRY_RUN = "--dry-run" in sys.argv
SPECIFIC_REG = None
for arg in sys.argv:
    if arg.startswith("--regulation="):
        SPECIFIC_REG = arg.split("=", 1)[1]

GLM_BASE_URL = "https://open.bigmodel.cn/api/coding/paas/v4"
REG_KB_NAME_PATTERN = "OJK%"

# ---------------------------------------------------------------- 系统 Prompt

SYSTEM_PROMPT = """\
You are an expert OJK (Otoritas Jasa Keuangan) regulatory analyst
specializing in Indonesian financial services fit-and-proper
assessment regulations.

Your task: Given structured sections (Pasal) of an OJK regulation,
extract EVERY testable compliance requirement as a structured
ChecklistItem. Your output will be reviewed line-by-line by an OJK
business expert before publication — accuracy and completeness
matter more than brevity.

STRICT RULES:
- R1: Extract ONLY requirements explicitly stated in the provided
  Pasal text. Never invent, infer beyond the text, or add
  requirements from your general knowledge.
- R2: For each requirement, you MUST quote the exact Pasal reference
  (Pasal number and ayat/huruf if applicable). If you cannot cite
  a Pasal, do not create the item.
- R3: Quote the key phrase from the original regulation text
  (pasal_text) verbatim.
- R4: If a Pasal contains multiple distinct testable requirements,
  split them into separate ChecklistItems, each citing the same Pasal.
- R5: If a Pasal is procedural/administrative (not a testable
  requirement on a candidate), skip it.
- R6: Classify compliance area strictly as one of:
  "Integrity" | "Financial Reputation" | "Competence" |
  "Structure" | "Completeness"
- R7: Classify severity strictly as one of:
  "critical" | "clarification" | "info"
- R8: Define check_method strictly as one of:
  "document_presence" | "cross_document" | "rule_computation"
- R9: evidence_type must be a concrete, nameable document or data
  source. Do not write vague values.
- R10: applicable_roles must be a non-empty subset of:
  ["Direktur", "Komisaris", "Direktur Utama", "Dewan Pengawas",
   "Pejabat Puncak", "Pemegang Saham Pengendali", "Eksekutif"]
  If applies to all, use ["*"].
- R11: keywords — extract 2-5 distinctive terms from regulation text.
- R12: Output ONLY valid JSON conforming to the schema. No markdown fences.

If a Pasal yields zero requirements, omit it silently.
"""

USER_TEMPLATE = """\
Regulation: {regulation_no}
Title: {regulation_title}
Source: {regulation_file}

--- PASAL SECTIONS ---
{pasal_slices}
--- END PASAL SECTIONS ---

Extract all testable requirements as JSON array of ChecklistItem.
"""

# ---------------------------------------------------------------- DB helpers

def get_db():
    if not PG_DSN:
        raise RuntimeError("WEKNORA_PG_DSN not set")
    return psycopg2.connect(PG_DSN)


def get_model_config(conn):
    with conn.cursor() as cur:
        cur.execute("""
            SELECT parameters->>'api_key' AS api_key,
                   parameters->>'base_url' AS base_url
            FROM models
            WHERE tenant_id = %s AND status = 'active'
            LIMIT 1
        """, (TENANT_ID,))
        row = cur.fetchone()
    if not row:
        raise RuntimeError(f"no active model for tenant {TENANT_ID}")
    return {"api_key": row[0], "base_url": row[1] or GLM_BASE_URL}


def get_regulation_kbs(conn):
    with conn.cursor() as cur:
        cur.execute("""
            SELECT DISTINCT kb.id, kb.name
            FROM knowledge_bases kb
            WHERE kb.tenant_id = %s AND kb.name LIKE %s
              AND kb.deleted_at IS NULL
        """, (TENANT_ID, REG_KB_NAME_PATTERN))
        return cur.fetchall()


def rebuild_text(conn, kb_id):
    with conn.cursor() as cur:
        cur.execute("""
            SELECT content, start_at, end_at
            FROM chunks
            WHERE knowledge_base_id = %s AND tenant_id = %s
              AND deleted_at IS NULL
              AND coalesce(metadata->>'sbk_method', '') != 'failed'
            ORDER BY start_at
        """, (kb_id, TENANT_ID))
        rows = cur.fetchall()
    if not rows:
        return None
    # 冲突检测
    for i in range(len(rows) - 1):
        for j in range(i + 1, len(rows)):
            a, b = rows[i], rows[j]
            if a[1] < b[2] and b[1] < a[2]:  # overlap
                if a[0] != b[0]:
                    raise RuntimeError(f"content conflict at overlap [{a[1]},{a[2]})")
    # 拼接
    parts = []
    cursor = 0
    for content, start, end in sorted(rows, key=lambda r: r[1]):
        if start < cursor:
            start = cursor
        if start >= end:
            continue
        parts.append(content)
        cursor = end
    return "".join(parts)


def slice_pasal(full_text, file_name, reg_name):
    penj_marker = "penjelasan atas peraturan"
    penj_start = full_text.lower().find(penj_marker)
    pasal_re = re.compile(
        r"^##\s+Pasal\s+(\d+)(?:\s+ayat\s+\((\d+)\))?(?:\s+huruf\s+([a-z]))?\s*$",
        re.MULTILINE | re.IGNORECASE,
    )
    matches = list(pasal_re.finditer(full_text))
    slices = []
    for i, m in enumerate(matches):
        num = m.group(1)
        ayat = m.group(2)
        huruf = m.group(3)
        label = f"Pasal {num}"
        if ayat:
            label += f" ayat ({ayat})"
        if huruf:
            label += f" huruf {huruf}"
        is_penj = penj_start >= 0 and m.start() > penj_start
        if is_penj:
            label += " (Penjelasan)"
        body_start = m.end()
        body_end = matches[i + 1].start() if i + 1 < len(matches) else len(full_text)
        body = full_text[body_start:body_end].strip()
        body = re.sub(r"<!--sbk:p\d+-b\d+-->", "", body)
        body = re.sub(r"^#{1,4}\s+", "", body, flags=re.MULTILINE)
        body = " ".join(body.split())
        if len(body) < 20:
            continue
        slices.append({"ref": label, "regulation": reg_name,
                        "file": file_name, "body": body, "is_penjelasan": is_penj})
    return slices


def call_glm(api_key, base_url, sys_prompt, user_prompt):
    payload = json.dumps({
        "model": "glm-5.3-flash",
        "messages": [
            {"role": "system", "content": sys_prompt},
            {"role": "user", "content": user_prompt},
        ],
        "max_tokens": 8192,
        "temperature": 0,
        "thinking": {"type": "disabled"},
    }).encode()
    req = urllib.request.Request(
        f"{base_url.rstrip('/')}/chat/completions",
        data=payload,
        headers={"Content-Type": "application/json",
                 "Authorization": f"Bearer {api_key}"},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=120) as resp:
        result = json.loads(resp.read())
    content = result["choices"][0]["message"]["content"]
    content = re.sub(r"^```json\s*", "", content)
    content = re.sub(r"\s*```\s*$", "", content)
    return json.loads(content)


def validate_item(item, all_refs):
    flags = []
    if item.get("area") not in {"Integrity", "Financial Reputation", "Competence", "Structure", "Completeness"}:
        flags.append("invalid_area")
    if item.get("severity") not in {"critical", "clarification", "info"}:
        flags.append("invalid_severity")
    if item.get("check_method") not in {"document_presence", "cross_document", "rule_computation"}:
        flags.append("invalid_check_method")
    pasal = item.get("pasal", "")
    if pasal and pasal not in all_refs:
        flags.append("pasal_unverified")
    if not item.get("applicable_roles"):
        item["applicable_roles"] = ["*"]
    return item, flags


# ---------------------------------------------------------------- main

def main():
    if not RUN_ID:
        print("OJK_RUN_ID not set; skipping", file=sys.stderr)
        return 0

    conn = get_db()
    try:
        cfg = get_model_config(conn)
        kbs = get_regulation_kbs(conn)
        if not kbs:
            print(f"No OJK KBs for tenant {TENANT_ID}", file=sys.stderr)
            return 0

        all_slices, all_refs = [], set()
        for kb_id, kb_name in kbs:
            if SPECIFIC_REG and SPECIFIC_REG not in kb_name:
                continue
            text = rebuild_text(conn, kb_id)
            if not text:
                continue
            reg_name = re.sub(r"\(OJK[^)]*\)", "", kb_name).strip()
            slices = slice_pasal(text, kb_name, reg_name)
            all_slices.extend(slices)
            all_refs.update(s["ref"] for s in slices)

        print(f"Slices: {len(all_slices)}", file=sys.stderr)

        batches, cur_batch, cur_size = [], [], 0
        for s in all_slices:
            sz = len(s["body"])
            if cur_size + sz > 12000 and cur_batch:
                batches.append(cur_batch); cur_batch = []; cur_size = 0
            cur_batch.append(s); cur_size += sz
        if cur_batch:
            batches.append(cur_batch)

        all_items, all_flags = [], []
        for i, batch in enumerate(batches):
            print(f"Batch {i+1}/{len(batches)}...", file=sys.stderr)
            pasals = "\n\n".join(f"{s['ref']}: {s['body']}" for s in batch)
            user = USER_TEMPLATE.format(regulation_no=batch[0]["regulation"],
                                         regulation_title=batch[0]["regulation"],
                                         regulation_file=batch[0]["file"],
                                         pasal_slices=pasals)
            if DRY_RUN:
                print(f"[DRY RUN] {len(user)} chars", file=sys.stderr)
                continue
            try:
                result = call_glm(cfg["api_key"], cfg["base_url"], SYSTEM_PROMPT, user)
                items = result.get("items", []) if isinstance(result, dict) else []
            except Exception as e:
                print(f"LLM error: {e}", file=sys.stderr); continue
            for item in items:
                v, f = validate_item(item, all_refs)
                all_items.append(v); all_flags.extend(f)

        # dedup
        seen, deduped = set(), []
        for item in all_items:
            rid = item.get("requirement_id")
            if rid and rid in seen:
                continue
            seen.add(rid); deduped.append(item)

        print(f"Items: {len(deduped)}, Flags: {len(all_flags)}", file=sys.stderr)

        with conn.cursor() as cur:
            now = time.time()
            for idx, item in enumerate(deduped, 1):
                item_id = f"c-{RUN_ID}-{idx:04d}"
                cur.execute("""
                    INSERT INTO ojk_checklist_items
                      (id, tenant_id, run_id, regulation, pasal, pasal_text,
                       area, requirement, requirement_id, evidence_type,
                       check_method, applicable_roles, severity, keywords,
                       source, _flag, status, reviewer_note, created_at, updated_at)
                    VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,now(),now())
                    ON CONFLICT (id) DO NOTHING
                """, (
                    item_id, TENANT_ID, RUN_ID,
                    item.get("regulation", ""), item.get("pasal", ""),
                    item.get("pasal_text", ""), item.get("area"),
                    item.get("requirement", ""), item.get("requirement_id"),
                    item.get("evidence_type"), item.get("check_method"),
                    json.dumps(item.get("applicable_roles", []), ensure_ascii=False),
                    item.get("severity", "info"),
                    json.dumps(item.get("keywords", []), ensure_ascii=False),
                    "penjelasan" if any("Penjelasan" in item.get("pasal", "") for _ in [1]) else "normal",
                    "; ".join(all_flags[:3]) if all_flags else None,
                    "pending",
                ))
            cur.execute("UPDATE ojk_runs SET status='done', total_items=%s, flagged_items=%s, updated_at=now() WHERE run_id=%s",
                        (len(deduped), len(all_flags), RUN_ID))
        conn.commit()
        print(f"Done: {len(deduped)} items, run={RUN_ID}", file=sys.stderr)
        return 0

    except Exception as e:
        print(f"ERROR: {e}", file=sys.stderr)
        try:
            with conn.cursor() as cur:
                cur.execute("UPDATE ojk_runs SET status='failed', error=%s, updated_at=now() WHERE run_id=%s",
                            (str(e)[:2000], RUN_ID))
            conn.commit()
        except Exception:
            pass
        return 1
    finally:
        conn.close()


if __name__ == "__main__":
    raise SystemExit(main())
