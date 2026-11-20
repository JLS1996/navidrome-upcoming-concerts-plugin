#!/usr/bin/env python3
"""Offline smoke checks for the concert dashboard, without external services."""
from pathlib import Path
from playwright.sync_api import sync_playwright
root=Path(__file__).resolve().parents[1]
with sync_playwright() as p:
    browser=p.chromium.launch(executable_path='/usr/bin/chromium',headless=True,args=['--no-sandbox'])
    for width in [320,768,1280]:
        page=browser.new_page(viewport={'width':width,'height':900})
        errors=[]
        page.on('pageerror',lambda e:errors.append(str(e)))
        page.set_content((root/'dist/preview.html').read_text())
        assert page.locator('article.concert').count()==3
        assert page.locator('article.concert:visible').count()==3
        page.locator('#search').fill('Demo Hall')
        assert page.locator('article.concert:visible').count()==1
        page.locator('#search').fill('')
        page.get_by_role('button',name='Calendar',exact=True).click()
        assert page.locator('#month-grid button').count()>=28
        assert page.evaluate('document.documentElement.scrollWidth <= innerWidth'),width
        page.screenshot(path=str(root/f'dist/preview-{width}.png'),full_page=True)
        assert not errors,errors
        page.close()
    page=browser.new_page(viewport={'width':320,'height':900})
    page.set_content((root/'dist/preview-ticketmaster.html').read_text())
    assert page.locator('article.concert').count()==4
    page.locator('#provider-status summary').click()
    assert 'Checked: no concerts found' in page.locator('#provider-status').inner_text()
    assert 'Artist identity unresolved' in page.locator('#provider-status').inner_text()
    assert 'Source: Ticketmaster' in page.locator('body').inner_text()
    assert 'Source: Skiddle' in page.locator('body').inner_text()
    assert page.get_by_role('link',name='Skiddle listing',exact=True).count()==1
    assert 'Source: SeatGeek' in page.locator('body').inner_text()
    assert page.get_by_role('link',name='SeatGeek listing',exact=True).count()==1
    assert page.get_by_role('link',name='Ticketmaster listing',exact=True).count()==1
    assert page.evaluate('document.documentElement.scrollWidth <= innerWidth')
    page.locator('#search').fill('Demo theatre')
    assert page.locator('article.concert:visible').count()==1
    page.screenshot(path=str(root/'dist/preview-ticketmaster-320.png'),full_page=True)
    page.close()
    browser.close()
print('Concert browser checks passed at 320, 768 and 1280 px')
