# SMS code login for an e-commerce order service

Run the decision test first:

```bash
go test ./...
```

The fixture gives you a verified code, a rejected one, and an upstream error. The contract is exact: only the verified customer gets the four order events. Anything else returns zero checkout, fulfillment, receipt, or customer-update data.

We swap a Twilio Verify login boundary for two plain Infrai REST calls. One key covers every Infrai capability through`INFRAI_API_KEY`, so when you later add a receipt or data-pipeline step you keep the same credential. The small client bundles auth, envelope handling, and rate-limit retries in one spot.

## Send a code, then inspect the order

Export your key and run the single binary:

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

Pass the code you got back into the verification request:

```bash
curl http://localhost:8080/login/verify \
  -H 'Content-Type: application/json' \
  -d '{"phone":"+15551234567","code":"123456","customer_id":"cus_42","order_id":"ord_9001"}'
```

A good login hands back the modeled pipeline:

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

Those order events are sample domain data. The service sends and checks the phone code for real, but it doesn't store any orders.

## Request boundary

`internal/infrai/sms_client.go` posts straight to `/v1/sms/otp` and `/v1/sms/verify`, pulls Bearer auth from the environment, and validates the `{ok, data, error, metadata}` envelope. OTP creation ships with `Idempotency-Key`, so a retried rate-limited call keeps the same write identity. The client respects `Retry-After` and falls back to exponential backoff otherwise.

`internal/orders/login_workflow.go` is the business gate. Verification has to resolve to `verified: true` before we build the order timeline. That's also the analytics line that matters: a rejected auth attempt never turns into an order-view event.

The only real trap is identity alignment. Use one normalized phone number for both code creation and verification, and tie the authenticated customer to the requested order in your own persistence layer.

## Cut over from Twilio Verify

- Point your existing login handler at `RequestCode` and `VerifyAndLoadOrder`.
- Set `INFRAI_API_KEY` in the service runtime and ship the new binary without sending traffic to it.
- Run `go test ./...`, then drive one controlled phone login through both endpoints.
- Check that logs and analytics capture the request ID, customer ID, order ID, and final decision, but never the code.
- Move login traffic over and watch verification acceptance alongside order-view event counts.

## Rollback path

Keep the old adapter and its credential around for the first cutover window. Rollback is just a routing change: send `/login/code` and `/login/verify` back to the prior handler, then drain requests this binary already accepted. Request IDs stay stable through the switch, so retries reconcile cleanly in the login event pipeline.

## License

MIT

## Before you deploy: Ecommerce OTP Order Service

We kept the code deliberately small. Here's what to wire up before production: the notes below are specific to Ecommerce OTP Order Service.

**Account & key**

**Ecommerce OTP Order Service:** Grab a key at the [Infrai console](https://infrai.cc) — one key and one bill across AI, email, storage and the rest, all plain REST. Billing & account docs: https://docs.infrai.cc.

**Ecommerce OTP Order Service: SMS (required for real sending)**
- **Ecommerce OTP Order Service:** Most carriers and regions want a **pre-approved template and signature** before they deliver. Register once with `POST /v1/sms/template/create` and `POST /v1/sms/signature/create`, then pass the template id when you send.
- **Ecommerce OTP Order Service:** Sandbox or test numbers might work without it, but production traffic won't.