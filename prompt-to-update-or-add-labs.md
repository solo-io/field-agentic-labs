# Prompt — Sync workshop labs with source repo changes

I keep two parallel directory trees in sync:

- **SOURCE_DIR**: a "scratch" demo directory where I prototype + iterate. Loose markdown, YAML, code, scripts — no lab structure.

- **WORKSHOP_DIR**: a polished, numbered lab series mirroring that scratch directory. Layout convention:

  ```
  README.md (categorized TOC, "Validated Versions" table, repo layout)
  000-overview.md
  001-..., 010-..., 020-..., ...
  099-cleanup.md
  appendix-*.md
  tracks/{install,demos,...}-track.md
  assets/   (only lab-specific YAML / code / images — NOT mirrored source trees)
  ```

Today the variables are:

```
SOURCE_DIR    = <paste path, e.g. /Users/michaellevan/gitrepos/agentic-demo-repo/substrate>
WORKSHOP_DIR  = <paste path, e.g. /Users/michaellevan/gitrepos/field-agentic-labs/agent-substrate>
SINCE         = <one of: "last sync" | a git ref like "main@{2.weeks.ago}" | a date "2026-06-15"
                 | a specific commit SHA. If unset, default to comparing every source file
                 against what's in the workshop.>
```

Your task: bring `WORKSHOP_DIR` up to date with what's in `SOURCE_DIR`.

---

## Phase 1 — Discover

1. **Map `SOURCE_DIR`** (depth 5+). For every markdown file, read it fully if < 300 lines, otherwise first 100 + last 50 + grep for `## ` headers. For every YAML, Dockerfile, `.tf`, `.py`, `.go`, `.sh`, `requirements.txt`, `package.json`: note path + line count + a one-sentence summary of what it does. Skip `vendor/`, `node_modules/`, `.terraform/`, `*.tfstate*`, `.git/`, `LICENSES/`.

2. **Map `WORKSHOP_DIR`**. List every numbered lab + appendix + track. For each, extract:
   - The "Source" section / mapping table from `README.md` if present
   - Which `SOURCE_DIR` files each lab visibly references (paths, asset includes, link-outs like `https://github.com/<upstream>/...`)

3. **Compute the diff** between Phase 1.1 and Phase 1.2. Bucket every `SOURCE_DIR` entry into:

   | Bucket | Meaning |
   |---|---|
   | **A. NEW** | Exists in `SOURCE_DIR`, not referenced by any lab |
   | **B. CHANGED** | Referenced by lab(s) but its content has shifted (use git log if `SINCE` was provided; otherwise compare current content against what the lab's code blocks / quoted sections show) |
   | **C. UNCHANGED** | Referenced and matches |
   | **D. REMOVED** | Referenced by a lab but no longer exists in `SOURCE_DIR` |

   Also bucket every `WORKSHOP_DIR` lab into:

   | Bucket | Meaning |
   |---|---|
   | **E. STALE** | References files that have CHANGED or been REMOVED |
   | **F. OK** | All its references are UNCHANGED |
   | **G. ORPHANED** | References files that no longer exist in `SOURCE_DIR` at all |

---

## Phase 2 — Plan

Before writing anything, present me with a numbered plan:

- For each **NEW** source file: propose either (a) which existing lab to extend, or (b) a new numbered lab slot (`051-...md`, `appendix-foo.md`, etc.) including the proposed title and where in the README TOC it goes.

- For each **CHANGED** source file: list which labs need editing and *what specifically* needs to change (version bumps, command output diffs, new flags, removed flags, schema changes).

- For each **REMOVED** source file: name the labs that need to drop their reference.

- Flag anything where you're not sure if it's an additive update or a breaking rewrite — ask me before deciding.

Use a question tool (or equivalent) to confirm:

- Recommended decisions for each NEW file (extend X vs create new lab number Y)
- Anything that affects the canonical paths (e.g. a new install method replacing the current canonical one in `040` — that's a big decision, surface it)
- Sensitive content in any new file (secrets, tfstate, private keys, hardcoded credentials) — confirm I want it skipped + parameterized

**Wait for my answers before proceeding.**

---

## Phase 3 — Execute

Once I confirm the plan:

1. **For each NEW lab**: write it in the existing workshop's style. Match the tone, the "Lab Objectives / Prerequisites / numbered steps / Next" structure, the callout patterns (`>` blockquotes for tips, tables for value-references, fenced code blocks with `bash`/`yaml` language tags).

2. **For each CHANGED lab**: edit in place. Keep edits minimal — don't rewrite paragraphs that didn't change. When updating command output blocks, paste the actual current output from the source (don't paraphrase).

3. **For NEW assets**: copy only the small lab-specific artifacts under `WORKSHOP_DIR/assets/`. Do NOT copy: vendored deps, license trees, terraform state, `terraform.tfvars`, `.env` files, private keys, tokens. For any asset that contained sensitive values in the source, parameterize with `${VAR}` placeholders and update the lab to walk the reader through substituting.

4. **Update `README.md`**:
   - Add new labs to the appropriate TOC section
   - Bump versions in the "Validated Versions" table if any changed
   - Update the "Repo Layout" tree
   - Refresh "Validated On" if cluster / chart / tool versions shifted

5. **Update `tracks/`** if any new lab belongs on a curated path.

6. **Update `099-cleanup.md`** if any new lab created resources that need cleanup steps.

---

## Phase 4 — Verify

After all edits, run a verification pass (use a single bash command for each):

- All inter-lab links resolve to existing files:

  ```bash
  grep -hoE '\]\(([0-9]{3}|appendix|tracks)[^)]*\.md(#[^)]*)?\)'
  ```

- All asset references resolve to existing files:

  ```bash
  grep -hoE '\]\(\.?\.?/?assets/[^)]+\)'
  ```

- Any Python source under `assets/` still compiles:

  ```bash
  python3 -m py_compile <path>
  ```

- Any YAML under `assets/` is still valid YAML:

  ```bash
  python3 -c "import yaml,sys; yaml.safe_load(open(sys.argv[1]))" <path>
  ```

- Print a final summary: N labs added, M labs updated, K assets copied, L assets parameterized, plus the file counts (labs / tracks / assets) and the link-check result.

---

## Constraints (follow strictly)

- **NEVER delete labs or sections without confirming with me** — even if a source file was removed, the lab may still be useful as historical context.

- **NEVER commit anything.** Show me the diff in the final summary; let me commit.

- **NEVER copy sensitive files.** Apply the same parameterize-and-document approach the existing workshops use (e.g. `terraform.tfvars.example`, `gateway-token.yaml` with `${SUBSTRATE_GATEWAY_TOKEN}`).

- **Match the existing workshop's voice** — direct, present tense, no marketing language, concrete commands over prose explanation.

- **If a CHANGED source file is now substantially different** from what its lab documents (e.g. an entire install path was rewritten upstream), DO NOT silently rewrite the lab — surface it in Phase 2 as a "needs human decision" item.

- **If you spot a previously-undocumented inconsistency in `SOURCE_DIR`** (two install paths that disagree, version pins that contradict each other), report it in the Phase 2 plan rather than picking one.

---

**Start with Phase 1. Report what you found before doing anything else.**
