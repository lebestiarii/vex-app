---
name: after-action-review
description: Standardizes the end-to-end Phase Planning, Model/Reasoning Sizing, User Walkthrough, After-Action Review (AAR), Context Handoff, and Git Worktree/Branch Merge & Prune rhythm for every project or feature chat.
---

# Phase Planning, Walkthrough & After-Action Review (AAR) Rhythm

Follow this skill at the **start** and **end** of every multi-step task, feature, or phase so planning, execution, verification, and handoffs maintain the exact same rhythm every time.

---

## Part 1: Pre-Phase Planning & Model Sizing (Run Before Coding)

Before modifying files on any non-trivial task, output a concise **Phase Plan** containing:

### 1. Phase → Story Breakdown
Break the work into **Phases**, and each phase into **Stories** (`X.1`, `X.2`, …). Each story must specify:
- **Intent** (1 sentence)
- **File Scope** (exact directories or files)
- **Acceptance Criteria** (testable outcome)

### 2. AI Model, Context & Reasoning Plan (Required Before Starting)
Present this table for every planned phase so the user can judge task intensity and switch models before execution:

| Phase | Intensity | Recommended Model & Reasoning | Context Footprint | Justification (Why Strong vs. Lighter Model) |
| :--- | :--- | :--- | :--- | :--- |
| Phase X | Low / Med / High | e.g., Gemini 4 Neon (High) or Gemini 4 Hydrogen (Medium) | Small / Medium / Large | State *why* this tier is needed (e.g., cross-package schema migration + concurrency vs. isolated UI styling or doc edits). |

**Model-Tier Selection Heuristic:**
- **Strong Model + High Reasoning** (e.g., Gemini 4 Neon — High): Architecture changes, database/schema migrations, concurrency/race debugging, security boundaries, multi-file refactors (>6 files), or ambiguous root-cause investigations.
- **Strong Model + Medium Reasoning**: Multi-component UI work, new feature stories with clear specs, test suite expansion.
- **Lighter/Fast Model + Low–Medium Reasoning** (e.g., Gemini 4 Hydrogen): Single-file tweaks, mechanical renames, JSON/data formatting, routine doc updates, or parallel regional/batch subagent tasks.

### 3. Git Isolation Check
State the target branch (`feat/<topic>`) and whether a `git worktree` is recommended (use a worktree when running parallel phases, risky refactors, or keeping the primary checkout runnable).

**Pause Gate:** End Part 1 by asking: *"Approve this plan (and switch to the recommended model if needed), and I will begin Story X.1."*

---

## Part 2: Phase-End Walkthrough & After-Action Review (Run After Verification Gate)

At the end of every phase—after running build/test gates and `.agent/skills/doc-drift-sweep/SKILL.md`—deliver these **7 sections in exact order**:

### 1. Verification Gate & Drift Sweep Results
- Report pass/fail and test counts for every gate command run (frontend typecheck/tests/build, Go `-race` tests, binary build).
- Summarize `.agent/skills/doc-drift-sweep/SKILL.md` findings (or confirm **"Zero drift detected"**).

### 2. Changes by Story
Group completed work by Story (`X.1`, `X.2`, …) with clickable `file://` links to all created, modified, or deleted files.

### 3. What Went Well
2–4 concrete bullets on what worked smoothly (e.g., clean abstractions, tests catching an edge case early, zero regressions, fast build).

### 4. What Went Poorly (Friction & Root Causes)
2–4 candid bullets on any stumbling blocks, rework, ambiguous requirements, flaky tests, tool failures, or spec/doc mismatches—and what caused them. If nothing went poorly, state *"No significant friction"* rather than inventing fluff.

### 5. Skill & Instruction Hygiene Recommendations
Explicitly evaluate `.agent/skills/`, `.agent/rules/`, and `AGENTS.md` based on what happened in this phase:
- **Add:** Any new reusable pattern or repeated pitfall that deserves a new skill.
- **Improve:** Any existing skill/rule that was outdated, incomplete, or contradicted the codebase.
- **Remove / Consolidate:** Any redundant, overlapping, or low-value skill/instruction that adds context bloat without preventing errors.

### 6. Context & Next-Step Handoff
Give a direct, actionable recommendation on **all three items**:
- **Context Action:** **Compact context** (if staying on the same tightly coupled feature with light history) vs. **Start a fresh chat** (strongly recommended after completing a full phase, crossing backend↔frontend boundaries, or when context is heavy with diffs/logs).
- **Next Model & Reasoning Level:** Name the exact model and reasoning level for the next phase/task, with a 1-sentence justification.
- **Handoff Prompt (if starting a fresh chat):** Provide a 3–5 line copy-paste starter prompt referencing the branch, completed phase, and next phase objective.

### 7. User Verification Walkthrough (Approval Gate)
Provide a short, step-by-step manual test script for the user:
1. Exact command to launch or build the app (e.g., `./bin/macos/vex`).
2. 3–6 specific clicks, queries, keyboard flows, or edge cases to verify visually/functionally.
3. End with: *"Please run through the walkthrough above and let me know if everything looks good. Once you approve, I will handle the merge and branch/worktree cleanup."*

---

## Part 3: Post-Approval Git Merge & Cleanup (Only After User Approves)

Do **not** merge or delete branches/worktrees until the user explicitly approves the walkthrough. Once approved:

1. **Commit Check:** Confirm working tree is clean on the active branch/worktree.
2. **Worktree → Feature Branch (when applicable):** If work was done in a `git worktree`, merge the worktree branch back into the parent `feat/<topic>` branch.
3. **Feature Branch → `main`:** Switch to `main`, merge `feat/<topic>` cleanly (fast-forward or `--no-ff` per user preference), and confirm `main` stays green. **Never `git push` unless explicitly requested.**
4. **Prune Recommendations:** Run `git worktree list` and `git branch --merged main`, then list stale worktrees and merged branches with exact cleanup commands (or execute them if the user approves):
   - `git worktree remove <path>` / `git worktree prune`
   - `git branch -d <merged-branch>`