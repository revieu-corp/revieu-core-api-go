# Payment state machine

`POST /orders/:id/pay` now creates or resumes a durable payment attempt keyed
by the `Idempotency-Key` request header. If the header is absent, the service
uses a deterministic order key so a repeated development request is still
idempotent. A successful replay returns the existing vouchers and never
decrements coupon inventory again.

Payment attempts use these states:

- `pending`: reserved for an attempt that has not started execution.
- `processing`: execution has started and may be recovered by reconciliation.
- `paid`: the order, payment, inventory decrement, and vouchers committed in
  one transaction.
- `failed`: execution ended without issuing vouchers; the order remains
  retryable.

`OrderService.ApplyPaymentCallback` is the gateway-agnostic callback contract.
A trusted integration can report `processing`, `paid`, or `failed` without
coupling the core domain to a specific PSP. A `paid` callback uses the same
server-side settlement transaction as the development mock path; a `failed`
callback records the reason and leaves the order retryable.

`OrderService.ReconcileStalePaymentAttempts` marks processing attempts older
than the supplied timeout as failed with
`stale_processing_timeout`. It is deliberately exposed as a service operation
so a deployment can schedule it with its existing job runner.

Apply migration `00009_add_payment_attempts.sql` before enabling the flow in a
shared environment. Migration `00008` is the operational audit table from the
observability PR.
