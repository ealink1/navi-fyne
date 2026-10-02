#!/usr/bin/env python3
"""Compose licensed Latin and CJK glyphs as OpenType/CFF resources.

Latin glyphs are decomposed and scaled into the CJK source font. No OS font is
redistributed. Derivatives have new family names and keep source license notices.
"""
import argparse
import io
from pathlib import Path

from fontTools.fontBuilder import FontBuilder
from fontTools.pens.recordingPen import DecomposingRecordingPen
from fontTools.pens.recordingPen import RecordingPen
from fontTools.pens.t2CharStringPen import T2CharStringPen
from fontTools.pens.transformPen import TransformPen
from fontTools.pens.ttGlyphPen import TTGlyphPen
from fontTools.ttLib import TTFont

ROOT = Path(__file__).resolve().parent.parent


def compose(cjk_path, latin_path, destination, family, compress=True):
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
    save_cff(base, destination, family, compress)


def save_cff(font, destination, family, compress=True):
    # go-text eagerly decodes every TrueType outline at font load. CFF keeps
    # compact charstrings and interprets the outlines of displayed glyphs only.
    # Keep the complete cmap, glyph order and advances; this is not subsetting.
    # Normalize the composed TrueType coordinates before conversion, matching
    # the rounding of the previous TTF assets instead of keeping Latin fractions.
    normalized = io.BytesIO()
    font.save(normalized)
    font = TTFont(io.BytesIO(normalized.getvalue()), recalcTimestamp=False)
    glyph_set = font.getGlyphSet()
    charstrings = {}
    for name in font.getGlyphOrder():
        # At 13 px / 1000 UPEM this quantization is below 0.007 px; it avoids
        # expanding every cubic control point into a five-byte real operand.
        pen = T2CharStringPen(font['hmtx'][name][0], glyph_set)
        glyph_set[name].draw(pen)
        charstrings[name] = pen.getCharString()
    for table in ('glyf', 'loca', 'prep', 'fpgm', 'cvt ', 'gasp'):
        if table in font:
            del font[table]
    font.sfntVersion = 'OTTO'
    builder = FontBuilder(font=font, isTTF=False)
    builder.setupCFF(family.replace(' ', '-'), {'FullName': family, 'FamilyName': family}, charstrings, {})
    builder.setupMaxp()
    builder.setupPost(keepGlyphNames=False)
    if compress:
        compact_charstrings(font)
    font.save(destination)


def compact_charstrings(font):
    """Share repeated CFF instructions, verifying every outline before saving."""
    import cffsubr

    reference = io.BytesIO()
    font.save(reference)
    original = TTFont(io.BytesIO(reference.getvalue()), recalcTimestamp=False)
    cffsubr.subroutinize(font, cff_version=1)
    if (original.getBestCmap() != font.getBestCmap()
            or original.getGlyphOrder() != font.getGlyphOrder()
            or original['hmtx'].metrics != font['hmtx'].metrics):
        raise RuntimeError('CFF compression changed coverage, ordering or metrics')
    before, after = original.getGlyphSet(), font.getGlyphSet()
    for name in original.getGlyphOrder():
        a, b = RecordingPen(), RecordingPen()
        before[name].draw(a)
        after[name].draw(b)
        if a.value != b.value:
            raise RuntimeError(f'CFF compression changed outline: {name}')


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--fyne-fonts', type=Path, required=True)
    parser.add_argument('--output', type=Path, default=ROOT / 'internal/ui/assets')
    parser.add_argument('--no-compress', action='store_true', help='generate an uncompressed diagnostic reference')
    args = parser.parse_args()
    cjk = ROOT / 'third_party/fonts'
    output = args.output
    compose(cjk / 'NotoSansSC-Regular.ttf', args.fyne_fonts / 'Inter-Regular.ttf',
            output / 'NaviUI-Regular.otf', 'NaviUI Regular', not args.no_compress)
    compose(cjk / 'NotoSansSC-Bold.ttf', args.fyne_fonts / 'NotoSans-Bold.ttf',
            output / 'NaviUI-Bold.otf', 'NaviUI Bold', not args.no_compress)
    compose(cjk / 'NotoSansSC-Regular.ttf', args.fyne_fonts / 'DejaVuSansMono-Powerline.ttf',
            output / 'NaviMono-Regular.otf', 'NaviMono Regular', not args.no_compress)


if __name__ == '__main__':
    main()
