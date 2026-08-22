# Operational observability baseline

The payment, merchant voucher redemption, and coupon lifecycle writes emit
structured JSON events through the existing logger. The stable fields are:

- `metric_name=revieu_transaction_total`, `metric_type=counter`
- `metric_name=revieu_transaction_duration_ms`, `metric_type=histogram`
- `action`, `result`, `error_class`, and `duration_ms`

The same business writes persist an entry in `operational_audit_logs` when the
table is available. Audit persistence is best effort and never changes the
business result. Successful audit rows include the actor, target, result,
duration, and a small JSON details object; failures classify the error as
`forbidden`, `not_found`, `validation`, `business_rule`, or `internal`.

## Migration

Apply `00008_add_operational_audit_logs.sql` with the normal Goose migration
workflow. Migration `00005` is reserved for merchant dishes, `00006` for
password-reset tokens, and `00007` for notification deduplication.

## Troubleshooting

Search application logs by `metric_name` and `action` first. Then query
`operational_audit_logs` by `action`, `result`, or `actor_id`. If the audit
table is not yet migrated, the application still completes the business write
and the structured log remains the source of operational evidence until the
migration is applied.
