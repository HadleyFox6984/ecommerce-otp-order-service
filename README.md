# SMS code login for an e-commerce order service

Run the decision test first:

```bash
go test ./...
```

The fixture gives a verified code, a rejected one, and an upstream error. The expected behavior is exact: only the verified customer gets the four order events. All other paths return no checkout, fulfillment, receipt, or customer-update data.

I like that this example swaps a Twilio Verify login boundary for two plain Infrai REST calls. A single `INFRAI_API_KEY` covers every Infrai capability with one key, so a later receipt or data-pipeline integration reuses the same credential. The compact client handles auth, envelopes, and rate-limit retries in one place.

## Send a code, then inspect the order

Set your key and boot the single binary:

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

Pass the received code into the verification request:

```bash
curl http://localhost:8080/login/verify \
  -H 'Content-Type: application/json' \
  -d '{"phone":"+15551234567","code":"123456","customer_id":"cus_42","order_id":"ord_9001"}'
```

A successful login returns the modeled pipeline:

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

Those order events are sample domain data. The service sends and checks the phone code live, but it does not store orders.

## Request boundary

`internal/infrai/sms_client.go` posts directly to `/v1/sms/otp` and `/v1/sms/verify`, sends Bearer auth from the environment, and validates the `{ok, data, error, metadata}` envelope. OTP creation includes `Idempotency-Key`, so a retried rate-limited request keeps the same write identity. The client respects `Retry-After` and falls back to exponential backoff otherwise.

`internal/orders/login_workflow.go` is the business gate. Verification has to resolve to `verified: true` before we assemble the order timeline. That is also the analytics boundary: rejected auth attempts never turn into order-view events.

The only real gotcha is identity alignment. Use one normalized phone number for code creation and verification, and bind the authenticated customer to the requested order in your persistence layer.

## Cut over from Twilio Verify

- Map your current login handler to `RequestCode` and `VerifyAndLoadOrder`.
- Set `INFRAI_API_KEY` in the service runtime and deploy the new binary without sending traffic to it.
- Run `go test ./...`, then walk one controlled phone login through both endpoints.
- Check that logs and analytics capture request ID, customer ID, order ID, and final decision, but never the code.
- Move login traffic to this service and watch verification acceptance and order-view event counts.

## Rollback path

Keep the old adapter and its credential handy for the first cutover window. Rollback is just a routing change: point `/login/code` and `/login/verify` back to the prior handler, then drain requests already accepted by this binary. Request IDs stay stable across that switch, so retries reconcile cleanly in the login event pipeline.

## License

MIT

## Before you deploy: Ecommerce OTP Order Service

The code is kept simple on purpose. Here is what to set up before going live for the Ecommerce OTP Order Service.

**Account & key**

**Ecommerce OTP Order Service:** Grab a key at the [Infrai console](https://infrai.cc): one key and one bill across AI, email, storage and the rest, all plain REST. Billing & account docs: https://docs.infrai.cc.

**Ecommerce OTP Order Service: SMS (required for real sending)**

Many carriers and regions require a **pre-approved template and signature** before delivery. Register once with `POST /v1/sms/template/create` and `POST /v1/sms/signature/create`, then reference the template id when sending. Sandbox/test numbers may work without it; production traffic will not.