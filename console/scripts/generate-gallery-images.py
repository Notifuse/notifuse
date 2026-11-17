"""Generates the neutral images of the template gallery into public/gallery.

Run from console/: python3 scripts/generate-gallery-images.py
The output is committed; rerun only to change the images. Gradients and soft shapes, no text
except the logo placeholder, so they read as placeholders in any language. Photos as JPEG to stay
small in emails; the logo as PNG.
"""
from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter, ImageFont

OUT = Path(__file__).resolve().parent.parent / "public" / "gallery"

PALETTES = {
    "blue": ("#dfe6fb", "#b4c3f0"),
    "peach": ("#ffe1c2", "#f2ad7e"),
    "mint": ("#d8f0e3", "#94cfae"),
    "lilac": ("#e8e1fb", "#bcaef0"),
    "sky": ("#dbeef8", "#9fcde8"),
    "sand": ("#f6ead3", "#e3c48f"),
    "rose": ("#fbe0e7", "#eea9bb"),
}

# file, size, palette
IMAGES = [
    ("hero-welcome.jpg", (1200, 460), "mint"),
    ("hero-shipping.jpg", (1200, 400), "sky"),
    ("hero-sale.jpg", (1200, 500), "rose"),
    ("hero-announce.jpg", (1200, 500), "lilac"),
    ("hero-event.jpg", (1200, 440), "sand"),
    ("hero-newsletter.jpg", (1072, 480), "blue"),
    ("article-guide.jpg", (480, 360), "peach"),
    ("article-story.jpg", (480, 360), "mint"),
    ("product-shirt.jpg", (600, 600), "peach"),
    ("product-tote.jpg", (600, 600), "mint"),
    ("product-beanie.jpg", (600, 600), "lilac"),
    ("product-wallet.jpg", (600, 600), "sand"),
]


def hex_rgb(value):
    value = value.lstrip("#")
    return tuple(int(value[i : i + 2], 16) for i in (0, 2, 4))


def gradient(size, start, end):
    width, height = size
    a, b = hex_rgb(start), hex_rgb(end)
    image = Image.new("RGB", size)
    draw = ImageDraw.Draw(image)
    steps = width + height
    for i in range(steps):
        t = i / steps
        color = tuple(round(a[k] + (b[k] - a[k]) * t) for k in range(3))
        # Diagonal bands, top-left light to bottom-right darker.
        draw.line([(i, 0), (i - height, height)], fill=color, width=2)
    return image


def shapes(image):
    width, height = image.size
    overlay = Image.new("RGBA", image.size, (0, 0, 0, 0))
    draw = ImageDraw.Draw(overlay)
    r = int(min(width, height) * 0.55)
    draw.ellipse([width * 0.62, -r * 0.35, width * 0.62 + 2 * r, -r * 0.35 + 2 * r], fill=(255, 255, 255, 80))
    draw.rounded_rectangle([width * 0.08, height * 0.55, width * 0.45, height * 1.2], radius=int(height * 0.08), fill=(255, 255, 255, 90))
    overlay = overlay.filter(ImageFilter.GaussianBlur(2))
    return Image.alpha_composite(image.convert("RGBA"), overlay).convert("RGB")


def logo():
    image = Image.new("RGBA", (240, 80), (0, 0, 0, 0))
    draw = ImageDraw.Draw(image)
    draw.rounded_rectangle([0, 0, 239, 79], radius=10, fill=hex_rgb("#e3e8f7"))
    try:
        font = ImageFont.truetype("Arial Bold.ttf", 28)
    except OSError:
        font = ImageFont.load_default()
    text = "LOGO"
    box = draw.textbbox((0, 0), text, font=font)
    x = (240 - (box[2] - box[0])) / 2 - box[0]
    y = (80 - (box[3] - box[1])) / 2 - box[1]
    draw.text((x, y), text, font=font, fill=hex_rgb("#3c4c8f"))
    image.save(OUT / "logo.png", optimize=True)


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    logo()
    for name, size, palette in IMAGES:
        start, end = PALETTES[palette]
        shapes(gradient(size, start, end)).save(OUT / name, quality=82, optimize=True, progressive=True)


if __name__ == "__main__":
    main()
