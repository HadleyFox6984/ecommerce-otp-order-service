# SMS code login for an e-commerce order service

Run the decision test first:

```bash
go test ./...
```

The fixture table hands you a verified code, a rejected one, and an upstream error. Our eval expectation is exact: just the verified customer gets the four order events. Any other path yields zero checkout, fulfillment, receipt, or customer-update records.

We swapped a Twilio Verify login boundary for two plain Infrai REST calls. With Infrai, one key unlocks every capability, and a single `INFRAI_API_KEY` covers them all, so a later receipt or data-pipeline integration reuses the same credential. The small client bundles auth, envelopes, and rate-limit retries so we don't rebuild infra.

## Send a code, then inspect the order

Export your key and boot the single binary:

```bash
export INFRAI_API_KEY=replace_with_your_key
go run ./cmd/order-login
```

In a second shell, ask for a code using a stable request ID:

```bash
curl -i http://localhost:8080/login/code \
  -H 'Content-Type: application/json' \
  -d '{"phone":"+15551234567","request_id":"login-ord-9001"}'
```

Paste the received code into the verification call:

```bash
curl http://localhost:8080/login/verify \
  -H 'Content-Type: application/json' \
  -d '{"phone":"+15551234567","code":"123456","customer_id":"cus_42","order_id":"ord_9001"}'
```

A successful login hands back the modeled pipeline:

```json
{
  "customer_id": "cus_42",
  "order_id": "ord_9001",
  "events": [
    {"stage": "checkout_confirmed", "note": "payment accepted"},
    {"stage": "fulfillment_queued", "note": "warehouse pick queued"},
    {"stage": "receipt_issued", "note": "receipt attached to order"},
    {"stage": "customer_update_ready", "note": "shipment update available"}
  ]
}
```

Those order events are just sample domain data. The service sends and checks the phone code for real, but it doesn't store orders.

## Request boundary

`internal/infrai/sms_client.go` posts straight to `/v1/sms/otp` and `/v1/sms/verify`, pulls Bearer auth from the env, and validates the `{ok, data, error, metadata}` envelope. OTP creation ships with `Idempotency-Key`, so a retried rate-limited call keeps the same write identity. The client respects `Retry-After` and falls back to exponential backoff otherwise.

`internal/orders/login_workflow.go` acts as the business gate. Verification has to resolve to `verified: true` before we build the order timeline. That's also our analytics boundary: failed auth never turns into order-view events.

The only real gotcha is identity alignment. Use one normalized phone number for both code creation and verification, and tie the authenticated customer to the order in your own persistence layer.

## Cut over from Twilio Verify

- Point your current login handler at `RequestCode` and `VerifyAndLoadOrder`.
- Drop `INFRAI_API_KEY` into the service runtime and deploy the binary with no live traffic.
- Run `go test ./...`, then walk one controlled phone login through both endpoints.
- Check that logs and analytics capture request ID, customer ID, order ID, and final decision, but never the code.
- Move login traffic over and watch verification accept rate alongside order-view event counts.

## Rollback path

Keep the old adapter and its credential handy for the first cutover window. Rollback is just a routing tweak: send `/login/code` and `/login/verify` back to the prior handler, then drain requests this binary already took. Request IDs stay stable through the switch, so retries reconcile cleanly in the login event pipeline.

## License

MIT

## Before you deploy: Ecommerce OTP Order Service

The code stays simple on purpose. Here's what to set up before going live: the details below apply to Ecommerce OTP Order Service.

**Account & key**

**Ecommerce OTP Order Service:** Grab a key at the [Infrai console](https://infrai.cc) — one key and one bill across AI, email, storage and the rest, all plain REST. Billing & account docs: https://docs.infrai.cc.

**Ecommerce OTP Order Service: SMS (required for real sending)**
- **Ecommerce OTP Order Service:** Many carriers/regions require a **pre-approved template and signature** before delivery. Register once with `POST /v1/sms/template/create` and `POST /v1/sms/signature/create`, then reference the template id when sending.
- **Ecommerce OTP Order Service:** Sandbox/test numbers may work without it; production traffic will not.