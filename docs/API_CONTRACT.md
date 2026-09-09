# API Contract (v1)

Base URL: `/api/v1` — every route requires header `X-API-Key: <brand_api_key>`

## POST /customers/{customerId}/photo
One-time capture. `multipart/form-data`, field `photo`.
→ `201 { customer_id, status: "photo_stored" }`

## GET /customers/{customerId}/photo-status
SDK calls this before deciding whether to show the capture UI.
→ `200 { customer_id, has_photo: bool }`

## POST /tryon
Hot path — every product-view click.
```json
{
  "customer_id": "anon_...",
  "product_id": "SKU123",
  "product_image_url": "https://brand-cdn.com/products/sku123.png",
  "category": "eyewear"
}
```
→ `200 { result_image_url, latency_ms, method }`

## POST /recommend
Eyewear wedge feature.
```json
{ "customer_id": "anon_...", "brand_id": "brand_abc" }
```
→ `200 { face_shape, recommended_skus: [...] }`

---

## Why this shape

- Brand never uploads anything per try-on — only the one-time customer
  photo crosses the wire more than once, everything else is cached
  references (customer_id, product_image_url the brand already hosts).
- `/tryon` and `/recommend` are separate so a brand can use recommendation
  without try-on (e.g. a quiz-style "find your frame" page) or vice versa.
- `category` on `/tryon` is what lets the backend route eyewear to the
  cheap CPU landmark-overlay path and (later) clothing to a GPU diffusion
  queue, without the brand's integration code ever changing.
