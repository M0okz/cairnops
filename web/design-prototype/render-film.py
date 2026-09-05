"""Rendu reproductible du film Svelte, 1920 × 1080, 30 images/s."""
import argparse
import json
import os
import subprocess
from pathlib import Path
from playwright.sync_api import sync_playwright

parser=argparse.ArgumentParser()
parser.add_argument('--preview',action='store_true')
args=parser.parse_args()
out=Path(os.environ.get('CAIRNOPS_DESIGN_OUTPUT','/Users/gregory.narcin/.codex/visualizations/2026/09/05/01a07286-fd7d-7d62-a1ae-be9db11f2315/cairnops-design'))
out.mkdir(parents=True,exist_ok=True)
with sync_playwright() as p:
    browser=p.chromium.launch(headless=True)
    page=browser.new_page(viewport={'width':1920,'height':1080},device_scale_factor=1)
    errors=[]
    page.on('pageerror',lambda e:errors.append(str(e)))
    page.goto('http://127.0.0.1:5186/film.html?capture=1')
    page.wait_for_load_state('networkidle')
    page.evaluate('document.fonts.ready')
    page.wait_for_function("typeof window.__setFilmTime === 'function'")
    if args.preview:
        for i,t in enumerate([2.8,8,14.5,22.5,26,36.5,41]):
            page.evaluate('(t)=>window.__setFilmTime(t)',t)
            page.screenshot(path=str(out/f'film-plan-{i+1}.png'))
            print(f'Plan {i+1} : {t} s',flush=True)
    else:
        destination=out/'CairnOps-presentation-et-developpement.mp4'
        log=(out/'film-encoding.log').open('w')
        command=['ffmpeg','-y','-hide_banner','-loglevel','warning','-f','image2pipe','-vcodec','mjpeg','-framerate','30','-i','pipe:0','-an','-c:v','libx264','-preset','medium','-crf','18','-vf','scale=out_color_matrix=bt709:out_range=tv,format=yuv420p','-movflags','+faststart','-color_primaries','bt709','-color_trc','bt709','-colorspace','bt709',str(destination)]
        encoder=subprocess.Popen(command,stdin=subprocess.PIPE,stderr=log)
        try:
            for frame in range(1275):
                page.evaluate('(t)=>window.__setFilmTime(t)',frame/30)
                encoder.stdin.write(page.screenshot(type='jpeg',quality=94))
                if frame%150==0: print(f'{frame}/1275 images',flush=True)
        finally:
            encoder.stdin.close()
        assert encoder.wait()==0,'Échec FFmpeg, voir film-encoding.log'
        log.close()
        print(str(destination),flush=True)
    assert not errors,errors
    browser.close()
