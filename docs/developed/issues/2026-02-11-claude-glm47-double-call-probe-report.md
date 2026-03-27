# 2026-02-11 Claude Code `glm-4.7` 首次/二次调用探测报告（只读核查）

## 1. 目标

验证终端 1（Claude Code）在以下固定配置下，是否存在“首次调用不稳定、二次调用恢复”的现象，并确认调用模型是否为 `glm-4.7`：

- API Key: `[REDACTED]`
- Base URL: `[REDACTED]`
- Model: `glm-4.7`

## 2. 与实际代码实现的对齐

本次临时脚本严格对齐 `crates/services/src/services/cc_switch.rs` 中 Claude 分支行为：

1. 使用隔离 `CLAUDE_HOME`
2. 写入 `config.json`，设置 `primaryApiKey = "any"`
3. 注入环境变量：
   - `ANTHROPIC_BASE_URL`
   - `ANTHROPIC_AUTH_TOKEN`
   - `ANTHROPIC_MODEL`
   - `ANTHROPIC_DEFAULT_HAIKU_MODEL`
   - `ANTHROPIC_DEFAULT_SONNET_MODEL`
   - `ANTHROPIC_DEFAULT_OPUS_MODEL`
4. 清除继承环境变量：`ANTHROPIC_API_KEY`
5. CLI 参数包含：`--dangerously-skip-permissions`

## 3. 实测结果

### 3.1 同一 Session 连续两次调用（`same-session`）

- 结果文件：`.tmp/claude-probe/20260211_223442/summary.json`
- 第一次调用：成功（约 15s）
- 第二次调用：失败（约 0.4s）
- 失败原因：`Session ID ... is already in use`

结论：同一 `session-id` 串行复用在该 CLI 模式下会触发会话占用错误，不适合作为“二次调用”探测方式。

### 3.2 新 Session 的二次调用（`new-session`）

- 结果文件：`.tmp/claude-probe/20260211_223508/summary.json`
- 第一次调用：超时（180s）
- 第二次调用：成功（约 15.6s）
- 成功返回：`PROBE_CALL_2_OK`
- `summary.json` 中 `modelUsage` 显示模型为：`glm-4.7`

结论：出现了“第一次异常、第二次恢复”的实测现象；成功调用时明确为 `glm-4.7`，无证据显示被切换到 `gpt-5.3-codex-xhigh` 执行终端任务。

## 4. 归因边界（基于本次证据）

1. 已证实调用链可成功打到 `glm-4.7`。
2. 已观察到首调用超时而二次成功的真实样本。
3. 本报告不直接断言上游服务必然有缺陷，但现象与“首调不稳定/冷启动/链路抖动”一致，需要更多轮次统计确认。

## 5. 建议的后续验证

1. 使用“新 session 二次调用”模式连续跑 10~30 轮，统计首调失败率与二调成功率。
2. 记录每轮 `duration_ms`、`timeout`、`return_code`、`modelUsage`。
3. 将统计报告给上游核对是否存在首调冷启动窗口。

---

## 6. 附录：本次临时测试脚本（原文）

```python
#!/usr/bin/env python
"""
临时探测脚本：验证 Claude Code 在指定上游(base_url/model/token)下
第一次调用与第一次完成后的第二次调用行为。

对齐当前代码实现（crates/services/src/services/cc_switch.rs 的 Claude 分支）：
1) 使用隔离 CLAUDE_HOME
2) 写入 config.json: {"primaryApiKey":"any"}
3) 注入环境变量：
   - ANTHROPIC_BASE_URL
   - ANTHROPIC_AUTH_TOKEN
   - ANTHROPIC_MODEL
   - ANTHROPIC_DEFAULT_HAIKU_MODEL
   - ANTHROPIC_DEFAULT_SONNET_MODEL
   - ANTHROPIC_DEFAULT_OPUS_MODEL
4) 清除继承的 ANTHROPIC_API_KEY
5) 使用 --dangerously-skip-permissions

说明：
- 本脚本走 `claude --print` 非交互模式，方便稳定对比首次/二次调用。
- 第二次调用可配置同 session 或新 session。
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import time
import uuid
from datetime import datetime, timezone
from pathlib import Path
from tempfile import gettempdir


def now_iso() -> str:
    return datetime.now(timezone.utc).isoformat()


def mask_secret(secret: str) -> str:
    if not secret:
        return ""
    if len(secret) <= 12:
        return secret
    return f"{secret[:8]}...{secret[-4:]}"


def ensure_claude_home(claude_home: Path) -> None:
    claude_home.mkdir(parents=True, exist_ok=True)
    config_path = claude_home / "config.json"

    config_obj = {}
    if config_path.exists():
        try:
            config_obj = json.loads(config_path.read_text(encoding="utf-8"))
            if not isinstance(config_obj, dict):
                config_obj = {}
        except Exception:
            config_obj = {}

    config_obj["primaryApiKey"] = "any"
    config_path.write_text(
        json.dumps(config_obj, ensure_ascii=False, indent=2),
        encoding="utf-8",
    )


def build_env(base_env: dict[str, str], *, base_url: str, api_key: str, model: str, claude_home: Path) -> dict[str, str]:
    env = dict(base_env)

    env["CLAUDE_HOME"] = str(claude_home)
    env["ANTHROPIC_BASE_URL"] = base_url
    env["ANTHROPIC_AUTH_TOKEN"] = api_key
    env["ANTHROPIC_MODEL"] = model
    env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = model
    env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = model
    env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = model

    env.pop("ANTHROPIC_API_KEY", None)
    return env


def run_once(
    *,
    env: dict[str, str],
    prompt: str,
    session_id: str,
    timeout_sec: int,
    out_dir: Path,
    label: str,
) -> dict:
    cmd = [
        "claude",
        "--print",
        "--output-format",
        "json",
        "--dangerously-skip-permissions",
        "--session-id",
        session_id,
        prompt,
    ]

    started_at = now_iso()
    start = time.perf_counter()
    try:
        proc = subprocess.run(
            cmd,
            env=env,
            text=True,
            capture_output=True,
            timeout=timeout_sec,
            encoding="utf-8",
            errors="replace",
        )
        duration = round(time.perf_counter() - start, 3)
        ended_at = now_iso()
        timed_out = False
        return_code = proc.returncode
        stdout = proc.stdout
        stderr = proc.stderr
    except subprocess.TimeoutExpired as exc:
        duration = round(time.perf_counter() - start, 3)
        ended_at = now_iso()
        timed_out = True
        return_code = -1
        stdout = (exc.stdout or "") if isinstance(exc.stdout, str) else ""
        stderr = (exc.stderr or "") if isinstance(exc.stderr, str) else ""

    (out_dir / f"{label}.stdout.txt").write_text(stdout, encoding="utf-8")
    (out_dir / f"{label}.stderr.txt").write_text(stderr, encoding="utf-8")

    parsed_json = None
    json_error = None
    if stdout.strip():
        try:
            parsed_json = json.loads(stdout)
        except Exception as e:
            json_error = str(e)

    return {
        "label": label,
        "started_at": started_at,
        "ended_at": ended_at,
        "duration_sec": duration,
        "timed_out": timed_out,
        "return_code": return_code,
        "stdout_path": str((out_dir / f"{label}.stdout.txt")),
        "stderr_path": str((out_dir / f"{label}.stderr.txt")),
        "stdout_preview": stdout[:500],
        "stderr_preview": stderr[:500],
        "stdout_is_json": parsed_json is not None,
        "stdout_json_parse_error": json_error,
        "stdout_json": parsed_json,
    }


def main() -> int:
    parser = argparse.ArgumentParser(description="Claude glm-4.7 first/second call probe")
    parser.add_argument("--api-key", required=True, help="ANTHROPIC_AUTH_TOKEN")
    parser.add_argument("--base-url", required=True, help="ANTHROPIC_BASE_URL")
    parser.add_argument("--model", required=True, help="ANTHROPIC_MODEL")
    parser.add_argument(
        "--prompt-1",
        default="请只回复字符串：PROBE_CALL_1_OK",
        help="第一次调用 prompt",
    )
    parser.add_argument(
        "--prompt-2",
        default="请只回复字符串：PROBE_CALL_2_OK",
        help="第二次调用 prompt",
    )
    parser.add_argument(
        "--timeout-sec",
        type=int,
        default=180,
        help="每次调用超时时间（秒）",
    )
    parser.add_argument(
        "--output-dir",
        default="",
        help="输出目录（默认写入仓库 .tmp/claude-probe/<timestamp>）",
    )
    parser.add_argument(
        "--second-call-mode",
        choices=["same-session", "new-session"],
        default="same-session",
        help="第二次调用复用同一 session 还是新建 session",
    )
    args = parser.parse_args()

    ts = datetime.now().strftime("%Y%m%d_%H%M%S")
    repo_root = Path(__file__).resolve().parents[2]
    out_dir = (
        Path(args.output_dir).resolve()
        if args.output_dir
        else (repo_root / ".tmp" / "claude-probe" / ts).resolve()
    )
    out_dir.mkdir(parents=True, exist_ok=True)

    claude_home = Path(gettempdir()) / "solodawn" / f"claude-probe-{ts}"
    ensure_claude_home(claude_home)

    env = build_env(
        os.environ,
        base_url=args.base_url,
        api_key=args.api_key,
        model=args.model,
        claude_home=claude_home,
    )

    session_id = str(uuid.uuid4())

    meta = {
        "created_at": now_iso(),
        "session_id": session_id,
        "base_url": args.base_url,
        "model": args.model,
        "api_key_masked": mask_secret(args.api_key),
        "claude_home": str(claude_home),
        "command_template": [
            "claude",
            "--print",
            "--output-format",
            "json",
            "--dangerously-skip-permissions",
            "--session-id",
            session_id,
            "<prompt>",
        ],
        "aligned_with_code": {
            "file": "crates/services/src/services/cc_switch.rs",
            "claude_envs": [
                "CLAUDE_HOME",
                "ANTHROPIC_BASE_URL",
                "ANTHROPIC_AUTH_TOKEN",
                "ANTHROPIC_MODEL",
                "ANTHROPIC_DEFAULT_HAIKU_MODEL",
                "ANTHROPIC_DEFAULT_SONNET_MODEL",
                "ANTHROPIC_DEFAULT_OPUS_MODEL",
            ],
            "unset_env": ["ANTHROPIC_API_KEY"],
            "auto_confirm_flag": "--dangerously-skip-permissions",
        },
    }

    first = run_once(
        env=env,
        prompt=args.prompt_1,
        session_id=session_id,
        timeout_sec=args.timeout_sec,
        out_dir=out_dir,
        label="call1",
    )

    second_session_id = session_id if args.second_call_mode == "same-session" else str(uuid.uuid4())

    second = run_once(
        env=env,
        prompt=args.prompt_2,
        session_id=second_session_id,
        timeout_sec=args.timeout_sec,
        out_dir=out_dir,
        label="call2",
    )

    summary = {
        "meta": meta,
        "second_call_mode": args.second_call_mode,
        "call2_session_id": second_session_id,
        "call1": first,
        "call2": second,
        "diagnosis": {
            "first_call_ok": (not first["timed_out"]) and first["return_code"] == 0,
            "second_call_ok": (not second["timed_out"]) and second["return_code"] == 0,
            "second_better_than_first": (
                ((first["timed_out"] or first["return_code"] != 0) and (not second["timed_out"]) and second["return_code"] == 0)
            ),
        },
    }

    summary_path = out_dir / "summary.json"
    summary_path.write_text(json.dumps(summary, ensure_ascii=False, indent=2), encoding="utf-8")

    print(json.dumps({
        "ok": True,
        "summary_path": str(summary_path),
        "first_call_ok": summary["diagnosis"]["first_call_ok"],
        "second_call_ok": summary["diagnosis"]["second_call_ok"],
        "second_better_than_first": summary["diagnosis"]["second_better_than_first"],
        "session_id": session_id,
        "api_key_masked": mask_secret(args.api_key),
    }, ensure_ascii=False, indent=2))

    return 0


if __name__ == "__main__":
    sys.exit(main())
```
