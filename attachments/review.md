Only the two bundled READMEs change. This corrects the description of existing behavior; it does not alter or authorize runtime operations. Review Architecture (bundled documentation and projection activation contract), Security (stored-runtime/reset authorization and data loss clarity), and Simplification.

Exact candidate sources:
- [tailapps/session-cost/README.md](tailapps/session-cost/README.md@d816892bb305f10194a7403427838f1d643e66b3:154)
- [tailapps/agent-guard/README.md](tailapps/agent-guard/README.md@d816892bb305f10194a7403427838f1d643e66b3:89)
- [internal/projection/projection.go](internal/projection/projection.go@d816892bb305f10194a7403427838f1d643e66b3:319)
- [docs/reference/resident-upgrade.md](docs/reference/resident-upgrade.md@d816892bb305f10194a7403427838f1d643e66b3:23)
- [docs/reference/cli.md](docs/reference/cli.md@d816892bb305f10194a7403427838f1d643e66b3:180)

The two current behavior artifacts at 7ecffd53013fd6ca45693a1a8e28b7c8d52432e8 have identical bytes at this candidate. No wording-only tests were added. All normal CI commands ran locally, including isolated demo; exact-head GitHub CI follows PR13.
