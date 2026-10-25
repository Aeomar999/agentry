# Shared settings for the reviewer-handoff measure scripts. Sourced, not run.
# Everything lives under .agentry/files/measure (git ignores it); relative paths only.
M=.agentry/files/measure
ADDR=127.0.0.1:7900
# The default team already uses the template's agent names, so Measure's get an m- prefix.
LEAD=m-lead; ENGINEER=m-engineer; REVIEWER=m-reviewer
