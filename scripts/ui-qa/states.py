"""Exercise UI-only states using local synthetic API responses."""

import argparse, json, re
from pathlib import Path
from playwright.sync_api import sync_playwright
from fixtures import install

p = argparse.ArgumentParser()
p.add_argument("--stage", default="states-before")
p.add_argument("--widths", default="1440,768,390,320")
p.add_argument("--only", default="")
p.add_argument("--height", type=int, default=900)
args = p.parse_args()
out = Path("/tmp/cairnops-alignment-evidence") / args.stage
out.mkdir(parents=True, exist_ok=True)


def button(name):
    return lambda page: page.get_by_role("button", name=re.compile(name)).first.click()


cases = [
    ("account", "/", button("^Compte$"), ".account .menu"),
    (
        "theme",
        "/",
        lambda page: page.locator(".theme-picker summary").click(),
        ".theme-panel",
    ),
    ("inbox", "/", button("^Notifications"), ".inbox .panel"),
    ("palette", "/", button("^Rechercher"), ".palette"),
    ("personalizer", "/", button("^Personnaliser"), ".personalizer"),
    ("incident", "/", button("^Ouvrir l.incident"), ".incident-drawer"),
    ("target-menu", "/cibles", button("^Ajouter une ressource"), ".add-panel"),
    (
        "new-target",
        "/cibles",
        lambda page: (button("^Ajouter une ressource")(page), button("^Créer")(page)),
        ".modal",
    ),
    (
        "reconciliation",
        "/cibles/rapprochements",
        button("^Rapprocher manuellement"),
        ".modal",
    ),
    (
        "software-detail",
        "/mises-a-jour",
        button("^Consulter les changements"),
        ".software-detail",
    ),
    ("maintenance", "/maintenance", button("^Planifier"), ".modal"),
    ("connector-chooser", "/connecteurs", button("^Ajouter un Connecteur"), ".modal"),
    ("connector-config", "/connecteurs", button("^Configurer$"), ".modal"),
    ("connector-suspension", "/connecteurs", button("^Suspendre$"), ".modal"),
    ("connector-removal", "/connecteurs", button("^Supprimer$"), ".modal"),
    ("mattermost", "/connecteurs", button("^Relier Mattermost"), ".modal"),
    ("new-account", "/reglages", button("^Ouvrir un compte"), ".modal"),
    (
        "device-pairing",
        "/reglages",
        lambda page: (
            button("^Associer un appareil")(page),
            page.locator(".qr-code").wait_for(),
        ),
        ".modal",
    ),
    (
        "oidc-advanced",
        "/reglages",
        lambda page: page.get_by_text("Options avancées", exact=True).click(),
        ".oidc-panel details[open]",
    ),
]
results = []
with sync_playwright() as pw:
    browser = pw.chromium.launch(headless=True)
    for theme in ["light", "dark"]:
        for width in map(int, args.widths.split(",")):
            for name, route, action, selector in cases:
                if args.only and name not in args.only.split(","):
                    continue
                context = browser.new_context(
                    viewport={"width": width, "height": args.height},
                    reduced_motion="reduce",
                )
                page = context.new_page()
                page.set_default_timeout(4000)
                publish_update = install(page, theme)
                errors = []
                page.on("pageerror", lambda e: errors.append(str(e)))
                try:
                    page.goto("http://127.0.0.1:5199" + route)
                    page.wait_for_load_state("networkidle")
                    publish_update()
                    page.evaluate(
                        "document.dispatchEvent(new Event('visibilitychange'))"
                    )
                    page.locator(".update-cta").wait_for()
                    action(page)
                    page.wait_for_load_state("networkidle")
                    page.locator(selector).first.wait_for()
                    # Open-state evidence includes all rendered panels, independently of component class names.
                    page.screenshot(
                        path=str(out / f"{name}-{theme}-{width}.png"), full_page=False
                    )
                    geometry = page.evaluate(
                        """() => {const visible=el=>el.checkVisibility({visibilityProperty:true})&&!el.closest('details:not([open])'); const rect=el=>{const r=el.getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height,right:r.right,bottom:r.bottom}};return {scroll:document.documentElement.scrollWidth,clippedActions:[...document.querySelectorAll('dialog[open] footer .btn,.modal footer .btn')].filter(visible).filter(el=>{const b=rect(el),p=rect(el.closest('dialog,.modal'));return b.bottom>p.bottom+1||b.right>p.right+1||b.x<p.x-1}).map(el=>el.textContent.trim()),panels:[...document.querySelectorAll('dialog[open],.resource-filter-panel,.modal,.palette,.account .menu,details[open] .theme-panel,.inbox .panel,[role=dialog]')].filter(visible).map(el=>({cls:el.className,...rect(el)})),spill:[...document.querySelectorAll('button,input,select,textarea')].filter(visible).filter(el=>!el.closest('.rail nav,.category-tabs,.tabs')).filter(el=>{const r=rect(el);return r.x< -1||r.right>innerWidth+1}).map(el=>({text:el.textContent.slice(0,45),cls:el.className,...rect(el)}))}}"""
                    )
                    result = {
                        "state": name,
                        "width": width,
                        "theme": theme,
                        "geometry": geometry,
                        "errors": errors,
                    }
                except Exception as e:
                    result = {
                        "state": name,
                        "width": width,
                        "theme": theme,
                        "errors": errors + [str(e)],
                    }

                if "geometry" in result:
                    g = result["geometry"]
                    if g["clippedActions"]:
                        result["errors"].append(
                            "Footer actions clipped: " + str(g["clippedActions"])
                        )
                    if g["scroll"] > width + 1 or g["spill"]:
                        result["errors"].append("Horizontal overflow")
                    if any(
                        b["x"] < -1
                        or b["right"] > width + 1
                        or b["y"] < -1
                        or b["bottom"] > args.height + 1
                        for b in g["panels"]
                    ):
                        result["errors"].append("Panel outside viewport")
                results.append(result)
                print(json.dumps(result), flush=True)
                context.close()
    browser.close()
(out / "geometry.json").write_text(json.dumps(results, indent=2))

raise SystemExit(int(any(row["errors"] for row in results)))
