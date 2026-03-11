create table incidents (
  id text primary key,
  service text not null,
  severity text not null,
  opened_at text not null,
  summary text not null,
  status text not null,
  suspected_deploy_id text,
  owner text not null
);

insert into incidents values
  ('inc-9001', 'checkout', 'sev1', '2026-03-06T10:18:00Z', 'Checkout jobs timing out after rollout', 'mitigated', 'dep-1005', 'alice'),
  ('inc-9002', 'billing', 'sev2', '2026-03-06T09:48:00Z', 'Invoice batch latency spike', 'monitoring', 'dep-1002', 'bob'),
  ('inc-9003', 'worker', 'sev3', '2026-03-06T08:35:00Z', 'Queue drain intermittently slow', 'open', 'dep-1003', 'cory'),
  ('inc-9004', 'checkout', 'sev2', '2026-03-06T10:24:00Z', 'Refund sync backlog growing', 'open', 'dep-1005', 'alice');
