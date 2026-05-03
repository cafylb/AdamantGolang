import json
import gzip
from pathlib import Path

input_file = Path("no_text.tgs")
output_file = Path("adamant_flag_simple.tgs")

with open(input_file, "rb") as f:
    data = gzip.decompress(f.read())
    lottie = json.loads(data)

bg_layer = {
    "ddd": 0,
    "ind": 0,
    "ty": 4,
    "nm": "Background",
    "sr": 1,
    "ks": {"o": {"a": 0, "k": 100}},
    "ao": 0,
    "shapes": [{
        "ty": "gr",
        "it": [
            {"ty": "rc", "s": {"a":0, "k": [512, 512]}, "p": {"a": 0, "k": [256, 256]}, "r": {"a": 0, "k": 0}},
            {"ty": "fl", "c": {"a": 0, "k": [0.0941, 0.1451, 0.2, 1]}, "o": {"a": 0, "k": 100}}
        ]
    }]
}

lottie["layers"].insert(0, bg_layer)

with gzip.open(output_file, "wb") as f:
    f.write(json.dumps(lottie, separators=(",", ":")).encode("utf-8"))

print(f"✅ Создан файл: {output_file}")
print(f"Размер: {output_file.stat().st_size} байт")