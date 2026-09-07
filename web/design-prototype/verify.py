"""Vérifications du prototype local, jamais de l'instance déployée."""
import json
import os
from pathlib import Path
from datetime import datetime, timezone
from playwright.sync_api import sync_playwright, expect

out = Path(os.environ.get('CAIRNOPS_DESIGN_OUTPUT', '/Users/gregory.narcin/.codex/visualizations/2026/09/05/01a07286-fd7d-7d62-a1ae-be9db11f2315/cairnops-design'))
out.mkdir(parents=True, exist_ok=True)
base = 'http://127.0.0.1:5186/'
report = {'screens': [], 'checks': [], 'errors': []}
with sync_playwright() as p:
    browser = p.chromium.launch(headless=True)
    context = browser.new_context(viewport={'width':1440,'height':1100}, timezone_id='Europe/Paris', reduced_motion='reduce')
    page = context.new_page()
    page.on('pageerror', lambda e: report['errors'].append(str(e)))
    for variant in ['A','B','C']:
        for theme in ['light','dark']:
            page.goto(f'{base}?variant={variant}&theme={theme}')
            page.wait_for_load_state('networkidle')
            expect(page.locator('html')).to_have_attribute('data-theme',theme)
            expect(page.get_by_role('heading',name='Vue d’ensemble',exact=True)).to_be_visible()
            name=f'desktop-{variant}-{theme}.png'
            page.screenshot(path=str(out/name),full_page=True)
            report['screens'].append(name)
    for width in [1024,768,390,320]:
        for theme in ['light','dark']:
            page.set_viewport_size({'width':width,'height':900})
            page.goto(f'{base}?variant=A&theme={theme}')
            page.wait_for_load_state('networkidle')
            overflow=page.evaluate('document.documentElement.scrollWidth > innerWidth')
            assert not overflow, f'Horizontal overflow: {width}/{theme}'
            name=f'width-{width}-{theme}.png'
            page.screenshot(path=str(out/name),full_page=True)
            report['screens'].append(name)
    report['checks'].append('A/B/C en clair/sombre, 320/390/768/1024/1440 px sans débordement')
    page.set_viewport_size({'width':1440,'height':1100})
    page.goto(f'{base}?theme=light')
    page.wait_for_load_state('networkidle')
    slider=page.get_by_role('slider',name='Explorer le temps de réponse')
    before=int(slider.get_attribute('aria-valuenow'))
    slider.focus();slider.press('ArrowLeft')
    assert int(slider.get_attribute('aria-valuenow'))==before-1
    page.get_by_role('radio',name='7 jours',exact=True).click()
    expect(page.get_by_role('radio',name='7 jours',exact=True)).to_have_attribute('aria-checked','true')
    report['checks'].append('Graphique : périodes et exploration clavier')
    page.get_by_role('radio',name='À surveiller 2',exact=True).click()
    assert page.locator('.p-table tbody tr').count()==2
    page.get_by_role('textbox',name='Rechercher une cible').fill('inexistant')
    expect(page.get_by_text('Aucune cible trouvée',exact=True)).to_be_visible()
    page.screenshot(path=str(out/'empty-light.png'),full_page=True)
    page.get_by_role('button',name='Afficher les cibles',exact=True).click()
    page.get_by_role('button',name='Page suivante').click()
    expect(page.get_by_text('6–10 sur 48 cibles',exact=True)).to_be_visible()
    report['checks'].append('Filtres, recherche, état vide et pagination')
    page.get_by_role('button',name='API publique',exact=True).click()
    expect(page.get_by_role('dialog')).to_be_visible()
    page.get_by_role('button',name='Acquitter l’incident',exact=True).click()
    expect(page.get_by_text('Pris en charge · supervision toujours active',exact=False)).to_be_visible()
    expect(page.get_by_role('dialog').get_by_text('Dégradée',exact=True)).to_be_visible()
    page.screenshot(path=str(out/'incident-light.png'),full_page=True)
    page.keyboard.press('Escape')
    expect(page.get_by_role('dialog')).not_to_be_visible()
    report['checks'].append('Détail : clavier, fermeture Échap, acquittement sans résolution')
    page.goto(base+'?theme=system')
    page.emulate_media(color_scheme='dark')
    expect(page.locator('html')).to_have_attribute('data-theme','dark')
    page.emulate_media(color_scheme='light')
    expect(page.locator('html')).to_have_attribute('data-theme','light')
    report['checks'].append('Thème système suit les changements de préférence')
    page.get_by_role('button',name='Choisir l’apparence',exact=True).click()
    page.get_by_role('menuitemradio',name='Solaire',exact=True).click()
    page.get_by_role('button',name='Choisir l’apparence',exact=True).click()
    expect(page.get_by_text('Choisis une ville. En attendant, le thème suit le système.',exact=False)).to_be_visible()
    page.get_by_role('menuitem',name='Ville : à choisir',exact=True).press('ArrowRight')
    page.get_by_role('menuitemradio',name='Paris',exact=True).click()
    page.get_by_role('button',name='Choisir l’apparence',exact=True).click()
    page.clock.install(time=datetime(2026,6,21,12,0,tzinfo=timezone.utc))
    page.evaluate("window.dispatchEvent(new Event('focus'))")
    expect(page.locator('html')).to_have_attribute('data-theme','light')
    page.clock.set_system_time(datetime(2026,6,21,23,0,tzinfo=timezone.utc))
    page.evaluate("window.dispatchEvent(new Event('focus'))")
    expect(page.locator('html')).to_have_attribute('data-theme','dark')
    page.screenshot(path=str(out/'solar-dark.png'),full_page=True)
    page.get_by_role('menuitem',name='Ville : Paris',exact=True).press('ArrowRight')
    page.get_by_role('menuitemradio',name='Tromsø',exact=True).click()
    page.get_by_role('button',name='Choisir l’apparence',exact=True).click()
    expect(page.get_by_text('Le soleil ne se couche pas aujourd’hui.',exact=True)).to_be_visible()
    expect(page.locator('html')).to_have_attribute('data-theme','light')
    page.clock.set_system_time(datetime(2026,12,21,12,0,tzinfo=timezone.utc))
    page.evaluate("window.dispatchEvent(new Event('focus'))")
    expect(page.get_by_text('Le soleil ne se lève pas aujourd’hui.',exact=True)).to_be_visible()
    expect(page.locator('html')).to_have_attribute('data-theme','dark')
    report['checks'].append('Solaire : ville explicite, jour/nuit, jours et nuits polaires, retour au premier plan')
    assert not report['errors'], report['errors']
    browser.close()
(out/'verification.json').write_text(json.dumps(report,ensure_ascii=False,indent=2))
print(json.dumps(report,ensure_ascii=False,indent=2))
