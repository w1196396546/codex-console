"""
Bit Browser 实测脚本：连接 CDP，分析 ChatGPT checkout 页面的支付按钮和完整交互流程。
"""

import json
import os
import re
import sys
import time
import traceback

# ---------- 配置 ----------
BITBROWSER_CDP_URL = "http://127.0.0.1:54937"
OUTPUT_DIR = os.path.dirname(os.path.abspath(__file__))
SCREENSHOT_PATH = os.path.join(OUTPUT_DIR, "checkout_screenshot.png")
REPORT_PATH = os.path.join(OUTPUT_DIR, "checkout_analysis_report.txt")

# Stripe iframe 选择器（与 browser_bind.py 一致）
_STRIPE_IFRAME_SELECTOR = 'iframe[name*="__privateStripeFrame"]'


def find_checkout_page(browser):
    """在所有 context/page 中查找 checkout 页面"""
    for ctx in browser.contexts:
        for page in ctx.pages:
            url = str(page.url or "")
            if "/checkout/" in url and "chatgpt.com" in url:
                return page
    return None


def analyze_stripe_iframes(page):
    """分析页面中所有 Stripe iframe"""
    results = []
    try:
        iframe_elements = page.query_selector_all(_STRIPE_IFRAME_SELECTOR)
        for i, el in enumerate(iframe_elements):
            try:
                box = el.bounding_box() or {}
                frame = el.content_frame()
                frame_url = str(frame.url if frame else "unknown")
                name = el.get_attribute("name") or ""

                iframe_type = "unknown"
                if "elements-inner-payment" in frame_url:
                    iframe_type = "Payment Element (卡片输入)"
                elif "elements-inner-address" in frame_url:
                    iframe_type = "Address Element (地址输入)"
                elif "hcaptcha" in frame_url.lower():
                    iframe_type = "HCaptcha"
                elif "controller" in frame_url:
                    iframe_type = "Stripe Controller"
                elif "m-outer" in frame_url:
                    iframe_type = "Stripe Metrics"

                results.append({
                    "index": i,
                    "type": iframe_type,
                    "name": name[:80],
                    "url": frame_url[:150],
                    "width": box.get("width", 0),
                    "height": box.get("height", 0),
                    "visible": box.get("height", 0) > 30 and box.get("width", 0) > 120,
                })
            except Exception as e:
                results.append({"index": i, "error": str(e)[:100]})
    except Exception as e:
        results.append({"error": f"query failed: {e}"})
    return results


def find_submit_button(page):
    """
    查找支付按钮（与 browser_bind.py _find_submit_button 完全一致的选择器优先级）
    """
    selectors = (
        '[data-testid="checkout-submit"]',
        'button[type="submit"]',
        'button:has-text("Subscribe")',
        'button:has-text("Pay")',
        'button:has-text("Confirm")',
        'button:has-text("Start trial")',
        'button:has-text("Claim")',
        'button:has-text("订阅")',
        'button:has-text("支付")',
        'button:has-text("确认")',
    )
    for selector in selectors:
        try:
            btn = page.query_selector(selector)
            if btn and btn.is_visible():
                text = (btn.text_content() or "").strip()[:100]
                html = btn.evaluate("el => el.outerHTML")[:500]
                return {
                    "found": True,
                    "selector": selector,
                    "text": text,
                    "outerHTML": html,
                    "disabled": btn.is_disabled(),
                    "bounding_box": btn.bounding_box(),
                }
        except Exception:
            continue
    return {"found": False}


def analyze_all_buttons(page):
    """获取页面上所有 button 元素的详细信息"""
    try:
        return page.evaluate("""() => {
            return Array.from(document.querySelectorAll('button')).map(btn => ({
                text: btn.textContent.trim().slice(0, 100),
                type: btn.type || '',
                id: btn.id || '',
                className: btn.className.slice(0, 200),
                disabled: btn.disabled,
                visible: btn.offsetParent !== null,
                dataTestId: btn.getAttribute('data-testid') || '',
                outerHTML: btn.outerHTML.slice(0, 400),
                rect: (() => {
                    const r = btn.getBoundingClientRect();
                    return {x: r.x, y: r.y, width: r.width, height: r.height};
                })()
            }));
        }""")
    except Exception as e:
        return [{"error": str(e)}]


def analyze_payment_element_fields(page):
    """分析 Payment Element iframe 内部字段结构"""
    iframe_elements = page.query_selector_all(_STRIPE_IFRAME_SELECTOR)
    for el in iframe_elements:
        try:
            frame = el.content_frame()
            if not frame or "elements-inner-payment" not in str(frame.url or ""):
                continue
            fields = frame.evaluate("""() => {
                const inputs = document.querySelectorAll('input');
                return Array.from(inputs).map(inp => ({
                    name: inp.name || '',
                    id: inp.id || '',
                    placeholder: inp.placeholder || '',
                    type: inp.type || '',
                    autocomplete: inp.autocomplete || '',
                    ariaLabel: inp.getAttribute('aria-label') || '',
                }));
            }""")
            return fields
        except Exception as e:
            return [{"error": str(e)[:200]}]
    return [{"error": "Payment Element iframe not found"}]


def analyze_address_element_fields(page):
    """分析 Address Element iframe 内部字段结构"""
    iframe_elements = page.query_selector_all(_STRIPE_IFRAME_SELECTOR)
    for el in iframe_elements:
        try:
            frame = el.content_frame()
            if not frame or "elements-inner-address" not in str(frame.url or ""):
                continue
            fields = frame.evaluate("""() => {
                const inputs = document.querySelectorAll('input, select');
                return Array.from(inputs).map(el => ({
                    tag: el.tagName.toLowerCase(),
                    name: el.name || '',
                    id: el.id || '',
                    placeholder: el.placeholder || '',
                    type: el.type || '',
                    ariaLabel: el.getAttribute('aria-label') || '',
                    options: el.tagName === 'SELECT'
                        ? Array.from(el.options).slice(0, 5).map(o => o.value)
                        : undefined,
                }));
            }""")
            return fields
        except Exception as e:
            return [{"error": str(e)[:200]}]
    return [{"error": "Address Element iframe not found"}]


def detect_challenge(page):
    """检测页面中是否存在 challenge（hCaptcha/3DS）"""
    challenge_info = {"detected": False, "types": []}
    try:
        body = str(page.evaluate("document.body ? document.body.innerText.slice(0, 3000) : ''") or "").lower()
        tokens = ("hcaptcha", "3d secure", "requires_action", "authentication required",
                  "verify you are human", "complete verification", "challenge")
        for token in tokens:
            if token in body:
                challenge_info["detected"] = True
                challenge_info["types"].append(f"text:{token}")
    except Exception:
        pass

    try:
        for frame in page.frames:
            url = str(getattr(frame, "url", "") or "").lower()
            if "hcaptcha" in url:
                challenge_info["detected"] = True
                challenge_info["types"].append("iframe:hcaptcha")
            if "3ds" in url:
                challenge_info["detected"] = True
                challenge_info["types"].append("iframe:3ds")
    except Exception:
        pass
    return challenge_info


def extract_stripe_params(page):
    """从页面 JS 中提取 Stripe 参数"""
    params = {}
    try:
        # 从 Stripe controller iframe URL 中提取
        for frame in page.frames:
            url = str(getattr(frame, "url", "") or "")
            pk_match = re.search(r"apiKey=(pk_(?:live|test)_[A-Za-z0-9]+)", url)
            if pk_match:
                params["publishable_key"] = pk_match.group(1)
            version_match = re.search(r"apiVersion=([^&]+)", url)
            if version_match:
                params["api_version"] = version_match.group(1)[:80]
    except Exception:
        pass

    try:
        title = page.title() or ""
        params["page_title"] = title
        params["page_url"] = page.url
    except Exception:
        pass
    return params


def capture_page_text_summary(page, max_len=2000):
    """获取页面文本摘要"""
    try:
        text = str(page.evaluate("document.body ? document.body.innerText : ''") or "")
        return text[:max_len]
    except Exception as e:
        return f"[error: {e}]"


def main():
    try:
        from playwright.sync_api import sync_playwright
    except ImportError:
        print("ERROR: playwright 未安装，请运行 pip install playwright && playwright install chromium")
        sys.exit(1)

    report_lines = []

    def log(msg):
        print(msg)
        report_lines.append(msg)

    log("=" * 70)
    log("Bit Browser Checkout 页面实测分析")
    log(f"时间: {time.strftime('%Y-%m-%d %H:%M:%S')}")
    log(f"CDP: {BITBROWSER_CDP_URL}")
    log("=" * 70)

    with sync_playwright() as p:
        log("\n[1/8] 连接 Bit Browser CDP...")
        try:
            browser = p.chromium.connect_over_cdp(BITBROWSER_CDP_URL)
            log(f"  ✅ 连接成功，contexts: {len(browser.contexts)}")
        except Exception as e:
            log(f"  ❌ 连接失败: {e}")
            sys.exit(1)

        # 列出所有页面
        log("\n[2/8] 扫描所有浏览器页面...")
        all_pages = []
        for ci, ctx in enumerate(browser.contexts):
            for pi, pg in enumerate(ctx.pages):
                url = str(pg.url or "")
                title = ""
                try:
                    title = pg.title() or ""
                except Exception:
                    pass
                log(f"  Context[{ci}] Page[{pi}]: {title[:40]} | {url[:100]}")
                all_pages.append(pg)

        # 查找 checkout 页面
        log("\n[3/8] 查找 Checkout 页面...")
        page = find_checkout_page(browser)
        if not page:
            log("  ❌ 未找到 checkout 页面！请确保 Bit Browser 中已打开 chatgpt.com/checkout/... 页面")
            # 尝试列出所有 URL
            for pg in all_pages:
                log(f"    -> {pg.url}")
            sys.exit(1)
        log(f"  ✅ 找到 checkout 页面: {page.url[:120]}")

        # 截图
        log("\n[4/8] 截图...")
        try:
            page.screenshot(path=SCREENSHOT_PATH, full_page=True)
            log(f"  ✅ 截图已保存: {SCREENSHOT_PATH}")
        except Exception as e:
            log(f"  ⚠️ 截图失败: {e}")

        # 提取 Stripe 参数
        log("\n[5/8] 提取 Stripe 参数...")
        stripe_params = extract_stripe_params(page)
        for k, v in stripe_params.items():
            log(f"  {k}: {v}")

        # 分析页面文本
        log("\n[6/8] 页面文本摘要...")
        page_text = capture_page_text_summary(page, 1500)
        log(f"  前500字符:\n  {page_text[:500]}")

        # 分析 Stripe iframes
        log("\n[7/8] Stripe iframe 分析...")
        iframes = analyze_stripe_iframes(page)
        for iframe in iframes:
            if "error" in iframe and "type" not in iframe:
                log(f"  ❌ {iframe['error']}")
            else:
                visible_flag = "✅可见" if iframe.get("visible") else "⬜隐藏"
                log(f"  [{iframe.get('index')}] {iframe.get('type', '?')} | {visible_flag} | "
                    f"{iframe.get('width', 0):.0f}x{iframe.get('height', 0):.0f}")

        # Payment Element 内部字段
        log("\n  --- Payment Element 内部字段 ---")
        pay_fields = analyze_payment_element_fields(page)
        for f in pay_fields:
            if "error" in f:
                log(f"  ❌ {f['error']}")
            else:
                log(f"    input: name={f.get('name')} placeholder={f.get('placeholder')} "
                    f"autocomplete={f.get('autocomplete')} aria-label={f.get('ariaLabel')}")

        # Address Element 内部字段
        log("\n  --- Address Element 内部字段 ---")
        addr_fields = analyze_address_element_fields(page)
        for f in addr_fields:
            if "error" in f:
                log(f"  ❌ {f['error']}")
            else:
                tag = f.get("tag", "?")
                name = f.get("name", "")
                placeholder = f.get("placeholder", "")
                log(f"    <{tag}>: name={name} placeholder={placeholder}")

        # 查找支付按钮 ⭐
        log("\n[8/8] 查找支付按钮（_find_submit_button 逻辑）...")
        submit_info = find_submit_button(page)
        if submit_info["found"]:
            log(f"  ✅ 支付按钮已找到！")
            log(f"    匹配选择器: {submit_info['selector']}")
            log(f"    按钮文案: {submit_info['text']}")
            log(f"    disabled: {submit_info['disabled']}")
            log(f"    位置: {submit_info.get('bounding_box')}")
            log(f"    outerHTML:\n    {submit_info['outerHTML']}")
        else:
            log("  ❌ 支付按钮未找到（所有选择器均未命中）")

        # 所有按钮
        log("\n  --- 页面所有 Button 元素 ---")
        all_buttons = analyze_all_buttons(page)
        for btn in all_buttons:
            if "error" in btn:
                log(f"  ❌ {btn['error']}")
            else:
                vis = "✅" if btn.get("visible") else "⬜"
                log(f"  {vis} text=[{btn.get('text', '')[:60]}] type={btn.get('type')} "
                    f"data-testid={btn.get('dataTestId')} disabled={btn.get('disabled')}")

        # Challenge 检测
        log("\n  --- Challenge 检测 ---")
        challenge = detect_challenge(page)
        if challenge["detected"]:
            log(f"  ⚠️ 检测到 Challenge: {', '.join(challenge['types'])}")
        else:
            log("  ✅ 未检测到 Challenge（页面正常）")

        log("\n" + "=" * 70)
        log("分析完成！")
        log("=" * 70)

        # 写入报告
        with open(REPORT_PATH, "w", encoding="utf-8") as f:
            f.write("\n".join(report_lines))
        print(f"\n报告已保存: {REPORT_PATH}")

        # 不要关闭浏览器连接（Bit Browser 是用户的）
        # browser.close()  # 不关闭！


if __name__ == "__main__":
    main()
