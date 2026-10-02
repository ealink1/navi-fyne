#!/usr/bin/env python3
"""Compose licensed Latin and CJK glyphs for Fyne's single-font text renderer.

Latin glyphs are decomposed and scaled into the CJK source font. No OS font is
redistributed. Derivatives have new family names and keep source license notices.
"""
import argparse
from pathlib import Path

from fontTools.pens.recordingPen import DecomposingRecordingPen
from fontTools.pens.transformPen import TransformPen
from fontTools.pens.ttGlyphPen import TTGlyphPen
from fontTools.ttLib import TTFont

ROOT = Path(__file__).resolve().parent.parent


def compose(cjk_path, latin_path, destination, family):
    base = TTFont(cjk_path, recalcTimestamp=False)
    latin = TTFont(latin_path, recalcTimestamp=False)
    latin_glyphs = latin.getGlyphSet()
    mapping, source = base.getBestCmap(), latin.getBestCmap()
    ratio = base['head'].unitsPerEm / latin['head'].unitsPerEm
    for codepoint, target in mapping.items():
        if codepoint > 0x2ff or codepoint not in source:
            continue
        glyph_name = source[codepoint]
        recording = DecomposingRecordingPen(latin_glyphs)
        latin_glyphs[glyph_name].draw(recording)
        pen = TTGlyphPen(None)
        recording.replay(TransformPen(pen, (ratio, 0, 0, ratio, 0, 0)))
        base['glyf'][target] = pen.glyph()
        advance, bearing = latin['hmtx'][glyph_name]
        base['hmtx'][target] = (round(advance * ratio), round(bearing * ratio))
    # Source Latin shaping rules refer to glyphs whose outlines were replaced.
    # Fyne uses horizontal glyph measurement; keeping those rules is incorrect.
    for table in ('GPOS', 'GSUB', 'GDEF'):
        if table in base:
            del base[table]
    for record in base['name'].names:
        if record.nameID in (1, 3, 4, 6, 16):
            record.string = family.encode(record.getEncoding())
    destination.parent.mkdir(parents=True, exist_ok=True)
    base.save(destination)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--fyne-fonts', type=Path, required=True)
    args = parser.parse_args()
    cjk = ROOT / 'third_party/fonts'
    output = ROOT / 'internal/ui/assets'
    compose(cjk / 'NotoSansSC-Regular.ttf', args.fyne_fonts / 'Inter-Regular.ttf',
            output / 'NaviUI-Regular.ttf', 'NaviUI Regular')
    compose(cjk / 'NotoSansSC-Bold.ttf', args.fyne_fonts / 'NotoSans-Bold.ttf',
            output / 'NaviUI-Bold.ttf', 'NaviUI Bold')
    compose(cjk / 'NotoSansSC-Regular.ttf', args.fyne_fonts / 'DejaVuSansMono-Powerline.ttf',
            output / 'NaviMono-Regular.ttf', 'NaviMono Regular')


if __name__ == '__main__':
    main()
