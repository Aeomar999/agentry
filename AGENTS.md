# Agentry

Agentry is a task board where the workers are AI agents: a coordinator agent hands each task from agent to agent until the work is checked and done.

This repo's behaviour is specified in `spec/`. Anyone who changes behaviour follows the "When you change behaviour" section of `spec/README.md`: the scenarios change in the same commit, and the report names the scenario IDs it touched, or says `no behaviour change`.

Run the checks with `make test` and `spec/check`.
