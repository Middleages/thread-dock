#!/usr/bin/env python3
"""Validate the runtime-facing project-template agent and Skill structure."""

from __future__ import annotations

import argparse
import re
import sys
import tomllib
from pathlib import Path

CANONICAL_SKILLS = (
    "coordinate-work",
    "develop-feature",
    "grill-plan",
    "tdd-task",
    "review-change",
    "publish-work",
)
LEGACY_SKILLS = (
    "implement-task",
    "open-agent-session",
    "plan-work",
    "record-work",
)
TEMPLATE_ROLES = (
    "td_coordinator",
    "td_docs_editor",
    "td_explorer",
    "td_feature_leader",
    "td_implementer",
    "td_reviewer",
)
ROOT_ROLES = ("td_coordinator", "td_implementer", "td_reviewer")


def fail(errors: list[str], path: Path, message: str) -> None:
    errors.append(f"{path}: {message}")


def parse_frontmatter(path: Path, errors: list[str]) -> tuple[dict[str, str], str]:
    """Parse the small YAML frontmatter subset consumed by OpenCode agent files."""
    try:
        lines = path.read_text(encoding="utf-8").splitlines()
    except OSError as exc:
        fail(errors, path, f"cannot read file ({exc})")
        return {}, ""
    if not lines or lines[0] != "---":
        fail(errors, path, "missing opening frontmatter delimiter")
        return {}, ""
    try:
        end = lines.index("---", 1)
    except ValueError:
        fail(errors, path, "missing closing frontmatter delimiter")
        return {}, ""

    fields: dict[str, str] = {}
    nested_parent: str | None = None
    nested_indent: int | None = None
    for line in lines[1:end]:
        if line[:1].isspace():
            if nested_parent != "permission":
                fail(errors, path, f"unexpected indented frontmatter line: {line!r}")
                continue
            indent = len(line) - len(line.lstrip(" "))
            if "\t" in line[: len(line) - len(line.lstrip())]:
                fail(errors, path, f"tabs are not allowed for permission entries: {line!r}")
                continue
            if nested_indent is None:
                nested_indent = indent
            child_pattern = " " * nested_indent + r"[A-Za-z][A-Za-z0-9_-]*:[ \t]*\S+"
            if indent != nested_indent or not re.fullmatch(child_pattern, line):
                fail(errors, path, f"invalid nested frontmatter line: {line!r}")
            continue
        match = re.fullmatch(r"([A-Za-z][A-Za-z0-9_-]*):(?:[ \t]*(.*))?", line)
        if match:
            key = match.group(1)
            value = (match.group(2) or "").strip()
            fields[key] = value
            nested_parent = key if key == "permission" and not value else None
            nested_indent = None
            continue
        fail(errors, path, f"invalid frontmatter line: {line!r}")

    return fields, "\n".join(lines[end + 1 :]).strip()


def normalize_prompt(prompt: str) -> str:
    """Compare the shared prompt while allowing Markdown code-span decoration."""
    prompt = re.sub(r"`([^`]+)`", r"\1", prompt)
    return "\n".join(line.rstrip() for line in prompt.strip().splitlines())


def check_toml_agents(root: Path, errors: list[str]) -> dict[str, str]:
    agents: dict[str, str] = {}
    directory = root / ".codex" / "agents"
    paths = sorted(directory.glob("*.toml")) if directory.is_dir() else []
    if not paths:
        fail(errors, directory, "no agent TOML files found")
    for role in ROOT_ROLES:
        if not (directory / f"{role}.toml").is_file():
            fail(errors, directory / f"{role}.toml", "required root role is missing")
    for path in paths:
        try:
            data = tomllib.loads(path.read_text(encoding="utf-8"))
        except (OSError, tomllib.TOMLDecodeError) as exc:
            fail(errors, path, f"invalid TOML ({exc})")
            continue
        name = data.get("name")
        if not isinstance(name, str) or not name:
            fail(errors, path, "name must be a non-empty string")
            continue
        if name != path.stem:
            fail(errors, path, f"name {name!r} does not match filename")
        for field in ("description", "developer_instructions"):
            if not isinstance(data.get(field), str) or not data[field].strip():
                fail(errors, path, f"{field} must be a non-empty string")
        for field in ("model", "model_reasoning_effort"):
            if field in data and (not isinstance(data[field], str) or not data[field].strip()):
                fail(errors, path, f"{field} must be a non-empty string when present")
        instructions = data.get("developer_instructions")
        if isinstance(instructions, str):
            agents[name] = instructions
    return agents


def check_template_agents(root: Path, errors: list[str]) -> None:
    codex_dir = root / "project-template" / ".codex" / "agents"
    opencode_dir = root / "project-template" / ".opencode" / "agents"
    for role in TEMPLATE_ROLES:
        codex_path = codex_dir / f"{role}.toml"
        opencode_path = opencode_dir / f"{role}.md"
        try:
            data = tomllib.loads(codex_path.read_text(encoding="utf-8"))
        except (OSError, tomllib.TOMLDecodeError) as exc:
            fail(errors, codex_path, f"invalid TOML ({exc})")
            data = {}
        if data.get("name") != role:
            fail(errors, codex_path, f"name must be {role!r}")
        for field in ("description", "developer_instructions"):
            if not isinstance(data.get(field), str) or not data[field].strip():
                fail(errors, codex_path, f"{field} must be a non-empty string")

        fields, body = parse_frontmatter(opencode_path, errors)
        if not fields.get("description"):
            fail(errors, opencode_path, "description must be present")
        if fields.get("mode") not in {"primary", "subagent"}:
            fail(errors, opencode_path, "mode must be primary or subagent")
        if not body:
            fail(errors, opencode_path, "prompt body must not be empty")
        instructions = data.get("developer_instructions")
        if isinstance(instructions, str) and body and normalize_prompt(instructions) != normalize_prompt(body):
            fail(errors, opencode_path, "Codex and OpenCode prompt bodies differ")


def check_skill(root: Path, name: str, errors: list[str]) -> None:
    path = root / "project-template" / ".agents" / "skills" / name / "SKILL.md"
    try:
        lines = path.read_text(encoding="utf-8").splitlines()
    except OSError as exc:
        fail(errors, path, f"cannot read file ({exc})")
        return
    if not lines or lines[0] != "---":
        fail(errors, path, "missing opening frontmatter delimiter")
        return
    try:
        end = lines.index("---", 1)
    except ValueError:
        fail(errors, path, "missing closing frontmatter delimiter")
        return
    fields: dict[str, str] = {}
    for line in lines[1:end]:
        match = re.fullmatch(r"([A-Za-z][A-Za-z0-9_-]*):[ \t]*(.*)", line)
        if match:
            fields[match.group(1)] = match.group(2).strip()
        else:
            fail(errors, path, f"invalid frontmatter line: {line!r}")
    if fields.get("name") != name:
        fail(errors, path, f"name must be {name!r}")
    if not fields.get("description"):
        fail(errors, path, "description must be present")
    for target in re.findall(r"\[[^\]]+\]\(([^)]+)\)", "\n".join(lines[end + 1 :])):
        target = target.strip().strip("<>").split("#", 1)[0]
        if not target or re.match(r"(?:[A-Za-z][A-Za-z0-9+.-]*:|//)", target):
            continue
        reference = (path.parent / target).resolve()
        if not reference.is_file():
            fail(errors, path, f"missing referenced file: {target}")


def check_skills(root: Path, errors: list[str]) -> None:
    for name in CANONICAL_SKILLS + LEGACY_SKILLS:
        check_skill(root, name, errors)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path("."), help="repository root (default: current directory)")
    args = parser.parse_args()
    root = args.root.resolve()
    errors: list[str] = []
    check_toml_agents(root, errors)
    check_template_agents(root, errors)
    check_skills(root, errors)
    if errors:
        for error in errors:
            print(f"template-check: {error}", file=sys.stderr)
        return 1
    print("template-check: agent and Skill structure is valid")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
