# Architecture Decision Records

One decision per file, `NNNN-short-title.md`, numbered in order. An accepted ADR is not
edited except for its status. To change a decision, write a new ADR that supersedes it and set
the old one's status to `Superseded by NNNN`. Use [template.md](template.md).

| ADR | Title | Status |
|-----|-------|--------|
| [0001](0001-record-architecture-decisions.md) | Record architecture decisions | Accepted |
| [0002](0002-go-stdlib-only.md) | Go, standard library only | Accepted |
| [0003](0003-per-user-scheduled-task.md) | Per-user collector started by a logon scheduled task | Accepted |
| [0004](0004-raw-facts-reclassify.md) | Log raw facts; the core re-classifies with the current config | Accepted |
| [0005](0005-credit-model.md) | Time credit model: capped backward credit, midnight split | Accepted |
| [0006](0006-two-measures.md) | Two measures: app usage and active gameplay | Accepted |
| [0007](0007-keyboard-mouse-only-mvp.md) | MVP input is keyboard and mouse only | Accepted |
