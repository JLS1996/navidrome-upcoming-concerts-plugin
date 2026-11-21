#!/usr/bin/env python3
"""Exercise the offline agenda, calendar and export in Chromium."""
from pathlib import Path
from shutil import which

from playwright.sync_api import expect, sync_playwright

ROOT = Path(__file__).resolve().parents[1]
DIST = ROOT / "dist"
DASHBOARD = (DIST / "preview-ticketmaster.html").read_text()


def open_report(browser, width, theme="dark"):
    page = browser.new_page(viewport={"width": width, "height": 1140})
    page.set_content(DASHBOARD.replace('data-theme="dark"', f'data-theme="{theme}"', 1))
    return page


with sync_playwright() as playwright:
    browser = playwright.chromium.launch(
        executable_path=which("chromium"), headless=True, args=["--no-sandbox"]
    )
    for width, theme in [(320, "dark"), (768, "bright"), (1280, "black")]:
        page = browser.new_page(viewport={"width": width, "height": 900})
        errors = []
        requests = []
        page.on("pageerror", lambda error: errors.append(str(error)))
        page.on("request", lambda request: requests.append(request.url))
        page.set_content(DASHBOARD.replace('data-theme="dark"', f'data-theme="{theme}"', 1))
        assert page.evaluate("document.documentElement.scrollWidth <= innerWidth"), width
        shows = page.locator("#agenda article.concert:visible")
        search = page.get_by_role("searchbox", name="Search")
        period = page.get_by_label("Timing or status")
        source = page.get_by_label("Source")
        clear = page.get_by_role("button", name="Clear filters")
        expect(shows).to_have_count(9)
        expect(shows.first).to_contain_text("Marigold Sessions")

        search.fill("paris cafe")
        expect(shows).to_have_count(1)
        expect(shows.first).to_contain_text("Café Marigold, Paris")
        expect(page.locator("#count")).to_have_text("1 concert")
        clear.click()
        expect(search).to_be_focused()
        period.select_option("timing:Upcoming")
        expect(shows).to_have_count(5)
        expect(shows.first).to_contain_text("Today")
        period.select_option("timing:Recent")
        expect(shows).to_have_count(2)
        expect(shows.first).to_contain_text("Cancelled")
        period.select_option("status:Cancelled")
        expect(shows).to_have_count(1)
        clear.click()

        # One merged listing is discoverable from each of its sources.
        search.fill("winter tour")
        for provider in ["SeatGeek", "Skiddle", "Ticketmaster"]:
            source.select_option(provider)
            expect(shows).to_have_count(1)
            expect(shows.first.get_by_role("link", name=f"{provider} listing")).to_be_visible()
        clear.click()

        page.get_by_role("button", name="Calendar", exact=True).click()
        grid = page.locator("#month-grid")
        expect(page.locator("#month-label")).to_have_text("November 2026")
        expect(grid.locator(".has-concerts")).to_have_count(6)
        expect(page.locator("#uncertain-list .uncertain-item")).to_have_count(1)
        expect(page.locator("#uncertain-list")).to_contain_text("Spring Sessions")
        expect(page.locator("#uncertain-list").get_by_role("link")).to_have_count(0)
        nov_30 = grid.locator('[data-date="2026-11-30"]')
        nov_30.click()
        expect(page.locator("#day-concert-list")).to_contain_text("Winter Residency")
        nov_30.press("ArrowRight")
        expect(page.locator("#month-label")).to_have_text("December 2026")
        expect(grid.locator('[data-date="2026-12-01"]')).to_be_focused()
        expect(page.locator("#day-concert-list")).to_contain_text("Through 2 December 2026")
        expect(page.locator("#uncertain-list .uncertain-item")).to_have_count(2)

        page.get_by_role("button", name="This month").click()
        search.fill("demo hall london")
        expect(page.locator("#month-label")).to_have_text("December 2026")
        expect(page.locator("#day-concert-list")).to_contain_text("A Place in the Morning")
        search.fill("nonexistent concert")
        expect(grid.locator(".has-concerts")).to_have_count(0)
        expect(page.locator("#uncertain-dates")).to_be_hidden()
        clear.click()
        assert page.evaluate("document.documentElement.scrollWidth <= innerWidth"), width

        # Browser filters leave the complete export intact.
        search.fill("demo hall")
        page.get_by_text("Export calendar", exact=True).click()
        page.get_by_text("Show calendar text", exact=True).click()
        calendar = page.locator("#calendar-text").input_value()
        assert calendar.count("BEGIN:VEVENT") == 7
        assert "DTEND;VALUE=DATE:20261203" in calendar
        assert "STATUS:CANCELLED" in calendar
        assert page.evaluate("document.documentElement.scrollWidth <= innerWidth"), width
        if width == 1280:
            with page.expect_download() as download:
                page.get_by_role("link", name="Download .ics").click()
            exported = download.value.path().read_bytes()
            assert download.value.suggested_filename == "music-concerts.ics"
            assert exported == calendar.replace("\n", "\r\n").encode()
        assert not errors, errors
        assert not any(url.startswith(("http:", "https:")) for url in requests), requests
        page.close()

    page = open_report(browser, 1280)
    page.screenshot(path=str(DIST / "dashboard.png"))
    page.get_by_role("button", name="Calendar", exact=True).click()
    page.locator('#month-grid [data-date="2026-11-29"]').click()
    page.screenshot(path=str(DIST / "calendar.png"), full_page=True)
    page.get_by_text("Lookup status", exact=True).click()
    page.locator("#provider-status summary").click()
    expect(page.locator("#provider-status")).to_contain_text("Checked: no concerts found")
    expect(page.locator("#provider-status")).to_contain_text("Artist identity unresolved")
    browser.close()

print("Concert agenda, calendar and exports passed in Chromium at 320, 768 and 1280 px.")
