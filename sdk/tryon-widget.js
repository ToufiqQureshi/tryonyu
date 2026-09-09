/**
 * Single script tag a brand pastes on their product page:
 *
 *   <script src="https://cdn.ourservice.com/tryon-widget.js"
 *           data-api-key="BRAND_API_KEY"
 *           data-product-id="SKU123"
 *           data-product-image="https://brand-cdn.com/products/sku123.png"
 *           data-category="eyewear"></script>
 *   <div id="tryon-widget-mount"></div>
 *
 * Core behavior implements the "photo once, reuse everywhere" decision:
 *   - customer_id persisted in localStorage (or brand's own logged-in
 *     customer id if they pass one via data-customer-id)
 *   - on load, check /photo-status
 *       -> has_photo true:  render "Try it on" button, one click, done
 *       -> has_photo false: render capture UI ONCE, then never again
 *          for this customer on this brand's site
 */
(function () {
  const script = document.currentScript;
  const API_BASE = "https://api.ourservice.com/api/v1"; // point at go-api
  const apiKey = script.getAttribute("data-api-key");
  const productId = script.getAttribute("data-product-id");
  const productImage = script.getAttribute("data-product-image");
  const category = script.getAttribute("data-category") || "eyewear";

  function getCustomerId() {
    const explicit = script.getAttribute("data-customer-id");
    if (explicit) return explicit;

    const key = "tryon_customer_id";
    let id = localStorage.getItem(key);
    if (!id) {
      id = "anon_" + crypto.randomUUID();
      localStorage.setItem(key, id);
    }
    return id;
  }

  async function apiFetch(path, options = {}) {
    const res = await fetch(API_BASE + path, {
      ...options,
      headers: { "X-API-Key": apiKey, ...(options.headers || {}) },
    });
    if (!res.ok) throw new Error("tryon api error: " + res.status);
    return res.json();
  }

  async function checkPhotoStatus(customerId) {
    return apiFetch(`/customers/${customerId}/photo-status`);
  }

  async function uploadPhoto(customerId, file) {
    const form = new FormData();
    form.append("photo", file);
    return apiFetch(`/customers/${customerId}/photo`, {
      method: "POST",
      body: form,
    });
  }

  async function runTryOn(customerId) {
    return apiFetch(`/tryon`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        customer_id: customerId,
        product_id: productId,
        product_image_url: productImage,
        category,
      }),
    });
  }

  function render(mount, html) {
    mount.innerHTML = html;
  }

  async function init() {
    const mount = document.getElementById("tryon-widget-mount");
    if (!mount) return;

    const customerId = getCustomerId();
    render(mount, `<button id="tryon-btn" disabled>Loading…</button>`);

    const status = await checkPhotoStatus(customerId);
    const btn = document.getElementById("tryon-btn");
    btn.disabled = false;

    if (status.has_photo) {
      // Returning customer on this brand's site: zero-friction path.
      btn.textContent = "Try it on";
      btn.onclick = async () => {
        btn.textContent = "Generating…";
        const result = await runTryOn(customerId);
        render(mount, `<img src="${result.result_image_url}" alt="Try-on result" />`);
      };
    } else {
      // First-ever try-on for this customer on this brand: the ONE
      // moment we ask for a photo. Never repeated after this.
      btn.textContent = "Take a photo to try this on";
      btn.onclick = () => {
        const input = document.createElement("input");
        input.type = "file";
        input.accept = "image/*";
        input.capture = "user";
        input.onchange = async (e) => {
          const file = e.target.files[0];
          if (!file) return;
          btn.textContent = "Setting up (one-time)…";
          await uploadPhoto(customerId, file);
          btn.textContent = "Try it on";
          btn.onclick = async () => {
            btn.textContent = "Generating…";
            const result = await runTryOn(customerId);
            render(mount, `<img src="${result.result_image_url}" alt="Try-on result" />`);
          };
        };
        input.click();
      };
    }
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
