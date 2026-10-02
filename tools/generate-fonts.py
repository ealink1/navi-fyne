#!/usr/bin/env python3
"""Create fixed weights from the licensed Noto Sans SC variable font.

Optional development tool: fonttools 4.63.0 was used for the committed assets.
Source: google/fonts, ofl/notosanssc/NotoSansSC[wght].ttf (SIL OFL 1.1).
"""
from pathlib import Path
import argparse
from fontTools.ttLib import TTFont
from fontTools.varLib.instancer import instantiateVariableFont

parser = argparse.ArgumentParser()
parser.add_argument('source',type=Path)
args = parser.parse_args()
target = Path(__file__).resolve().parent.parent/'internal/ui/assets'
for weight,label in [(400,'Regular'),(600,'Bold')]:
    font = instantiateVariableFont(TTFont(args.source),{'wght':weight},inplace=True)
    font.save(target/f'NotoSansSC-{label}.ttf')
    print(f'Generated fixed weight {weight}: {label}')
