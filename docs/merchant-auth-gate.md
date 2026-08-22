# Merchant eligibility gate

The merchant write surface now has one HTTP authorization gate in
`internal/authorization/merchant.go`.

## State contract

- No merchant record: merchant onboarding is required (`403`).
- Inactive merchant record: merchant operations are blocked (`403`).
- `unverified`, `pending`, or `rejected`: account-level reads and draft
  onboarding are allowed, but publish-grade actions are blocked (`403`).
- `verified`: publish-grade merchant actions are allowed after the existing
  ownership checks run.

## Route policy

`POST /merchant/stores` remains the onboarding entry point and may create a
draft store/merchant record. The following operations require an existing
active merchant account:

- merchant store list, update, deactivate, and delete;
- merchant-owned coupon list.

The following operations require a verified merchant:

- store activation;
- coupon create, update, enable, disable, and delete;
- dish create, update, delete, enable, and disable;
- merchant voucher scan and redemption;
- merchant account deletion.

This keeps the gray-release onboarding path usable while preventing an
unverified account from publishing purchasable inventory or redeeming a
customer voucher. The verification endpoints remain outside the gate so a
merchant can submit and check its own verification state.

## Compatibility and rollout

Existing users can enter onboarding through `POST /merchant/stores` or the
verification endpoints. Existing stores/coupons remain readable through
public routes. Environments using `AUTO_VERIFY_NEW_MERCHANTS=true` continue to
promote newly created merchants to `verified` for demo fixtures; production
should use the normal verification review path instead.
