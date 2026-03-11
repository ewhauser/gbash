# On-call Handoff

- Checkout looked healthy until `dep-1005`, then queue-backed jobs started timing out within five minutes.
- Billing had a latency blip earlier in the morning, but no broad customer impact yet.
- Worker lag appears correlated with downstream congestion, not a new deploy.
- If you need a short report for leadership, write it under `/home/agent/work` so later questions can reuse it.
