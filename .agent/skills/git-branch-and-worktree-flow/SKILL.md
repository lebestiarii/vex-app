---
name: git-branch-and-worktree-flow
description: Standardized Git isolation, worktree management, post-approval merge ordering, and branch/worktree pruning discipline across all repositories.
---

# Git Branch, Worktree & Post-Approval Merge Discipline

Follow this procedure on every code change to keep `main` clean and avoid accidental merges or workspace clutter.

## 1. Isolation Before Coding
- **Default:** Develop on a descriptive local branch (`feat/<topic>`, `fix/<topic>`, `refactor/<topic>`). Never commit directly to `main`.
- **When to Use `git worktree`:** Create a worktree when running parallel phases, testing risky refactors, or keeping the primary checkout runnable while developing.
  - Pattern: `git worktree add ../<repo>-<topic> -b feat/<topic>`

## 2. Hard Approval Gate
- **Never merge** a worktree branch into a feature branch, **never merge** a feature branch into `main`, and **never run `git push`** until the user explicitly approves the verification walkthrough.

## 3. Post-Approval Merge Sequence
Once the user approves:
1. Verify working tree status is clean (`git status`) and all changes are committed on the active branch/worktree.
2. **Worktree → Feature Branch (when applicable):** Merge the worktree branch cleanly back into its parent feature branch.
3. **Feature Branch → `main`:** Switch to `main`, merge the feature branch cleanly, and confirm `main` passes a quick status/test sanity check.
4. Do **not** push to remote unless explicitly instructed.

## 4. Prune & Cleanup Recommendations
Immediately after merging, inspect:
- `git worktree list`
- `git branch --merged main`

Present any stale worktrees or fully merged local branches with ready-to-run cleanup commands (or execute them upon confirmation):
- `git worktree remove <path>` and `git worktree prune`
- `git branch -d <branch-name>`