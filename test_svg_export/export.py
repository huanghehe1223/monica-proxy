import cairosvg

cairosvg.svg2png(
    url="test_svg_export/final.svg",
    write_to="test_svg_export/final.png"
)

print("转换完成：final.svg -> final.png")