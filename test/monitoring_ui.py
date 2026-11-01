"""Opt-in browser check against a synthetic local Core started with -demo."""
import json
import pathlib
import sys
import urllib.request

from playwright.sync_api import sync_playwright

base = sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:18081"
with urllib.request.urlopen(base + "/version", timeout=5) as response:
    assert json.load(response)["demo"] is True, "This check only runs against an explicit demo Core"

errors = []
with sync_playwright() as playwright:
    browser = playwright.chromium.launch(headless=True)
    page = browser.new_page(viewport={"width": 1440, "height": 1100})
    page.on("pageerror", lambda error: errors.append(str(error)))
    page.goto(base)
    page.locator('[name="username"]').fill("admin")
    page.locator('[name="password"]').fill("subgate-demo")
    page.locator('#login-form button[type="submit"]').click()
    page.locator('[data-route="analysis"]').wait_for()
    page.goto(base + "/#/analysis")
    page.locator('[data-action="edit-analysis"]').click()
    page.locator('[name="shaper_mbps"]').fill("10")
    page.locator('[name="shaper_minutes"]').fill("5")
    page.locator('[name="restricted_protocols"]').fill("BitTorrent")
    page.locator('[name="allowlist"]').fill("synthetic-exempt-account")
    page.locator('[name="automatic_shaper"]').select_option("true")
    with page.expect_response(lambda response: response.url.endswith("/analysis/policy") and response.request.method == "PUT") as saved:
        page.locator('#modal-form button[type="submit"]').click()
    assert saved.value.status == 200, saved.value.text()
    page.get_by_text("Автоматика включена", exact=True).wait_for()
    page.get_by_text("Коллекторы не подключены", exact=True).wait_for()
    assert page.get_by_text("Команд пока нет", exact=True).is_visible()
    page.locator('#toast.visible').wait_for(state="hidden")
    directory = pathlib.Path("docs/screenshots")
    directory.mkdir(parents=True, exist_ok=True)
    page.screenshot(path=str(directory / "monitoring-analysis.png"), full_page=True, animations="disabled")
    page.set_viewport_size({"width": 390, "height": 844})
    page.screenshot(path=str(directory / "monitoring-analysis-mobile.png"), full_page=True, animations="disabled")
    assert page.evaluate("document.documentElement.scrollWidth <= window.innerWidth + 2"), "mobile horizontal overflow"
    page.locator('[data-action="edit-analysis"]').click()
    assert page.locator('[name="shaper_mbps"]').input_value() == "10"
    assert page.locator('[name="restricted_protocols"]').input_value() == "BitTorrent"
    assert page.locator('[name="allowlist"]').input_value() == "synthetic-exempt-account"
    page.locator('[name="automatic_shaper"]').select_option("false")
    page.locator('#modal-form button[type="submit"]').click()
    page.get_by_text("Автоматика выключена", exact=True).wait_for()
    browser.close()
assert not errors, errors
print("Monitoring browser settings round-trip, explicit automation, desktop/mobile rendering: PASS")
