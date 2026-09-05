"""Captures du prototype local pour le film, pas de l'instance réelle."""
from pathlib import Path
from playwright.sync_api import sync_playwright

out=Path(__file__).parent/'film-assets'
out.mkdir(exist_ok=True)
with sync_playwright() as p:
    browser=p.chromium.launch(headless=True)
    page=browser.new_page(viewport={'width':1440,'height':1040},device_scale_factor=1,reduced_motion='reduce')
    for theme in ['light','dark']:
        page.goto(f'http://127.0.0.1:5186/?variant=A&theme={theme}')
        page.wait_for_load_state('networkidle')
        page.add_style_tag(content='.p-prototype-bar,.p-demo-label,.p-page-actions{visibility:hidden!important}')
        page.screenshot(path=str(out/f'overview-{theme}.png'))
    page.locator('.p-incident-title').filter(has_text='API publique').click()
    page.get_by_role('dialog').screenshot(path=str(out/'incident.png'))
    browser.close()
print(str(out))
