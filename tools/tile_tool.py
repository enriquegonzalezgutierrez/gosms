import json
import os
import sys
from PIL import Image

def unpack(rom_name):
    orig_sheet = f"assets/{rom_name}/original/spritesheet.png"
    manifest_path = f"assets/{rom_name}/original/manifest.json"
    out_dir = f"assets/{rom_name}/ai_workspace"
    
    os.makedirs(out_dir, exist_ok=True)
    
    im = Image.open(orig_sheet)
    with open(manifest_path, 'r') as f:
        manifest = json.load(f)
        
    for item in manifest:
        idx = item['index']
        is_sprite = item.get('is_sprite', False)
        prefix = "spr" if is_sprite else "bg"
        
        gx = (idx % 32) * 12 + 2
        gy = (idx // 32) * 12 + 2
        
        # Extraemos solo el tile útil de 8x8
        tile = im.crop((gx, gy, gx+8, gy+8))
        tile.save(os.path.join(out_dir, f"{prefix}_{idx:04d}.png"))
        
    print(f"✅ {len(manifest)} tiles extraídos a {out_dir}/")
    print("👉 Ahora puedes pasar esta carpeta por ComfyUI / Stable Diffusion (Img2Img).")

def pack(rom_name):
    ai_dir = f"assets/{rom_name}/ai_workspace"
    manifest_path = f"assets/{rom_name}/original/manifest.json"
    hd_dir = f"assets/{rom_name}/hd"
    hd_sheet = f"{hd_dir}/spritesheet.png"
    
    os.makedirs(hd_dir, exist_ok=True)
    
    with open(manifest_path, 'r') as f:
        manifest = json.load(f)
        
    cols = 32
    rows = (len(manifest) + cols - 1) // cols
    if rows == 0: rows = 1
    
    # Escala 4x: celda de 48x48 (padding de 8px por lado, tile de 32x32)
    sheet = Image.new("RGBA", (cols * 48, rows * 48), (0,0,0,0))
    
    for item in manifest:
        idx = item['index']
        is_sprite = item.get('is_sprite', False)
        prefix = "spr" if is_sprite else "bg"
        
        tile_path = os.path.join(ai_dir, f"{prefix}_{idx:04d}.png")
        if not os.path.exists(tile_path):
            continue
            
        t_img = Image.open(tile_path).convert("RGBA")
        if t_img.size != (32, 32):
            t_img = t_img.resize((32, 32), Image.NEAREST)
            
        gx = (idx % 32) * 48
        gy = (idx // 32) * 48
        
        # Pegar en el centro (padding 8)
        sheet.paste(t_img, (gx+8, gy+8))
        
        # Gutter Clamping (Anti-Bleed) para evitar bordes blancos/negros
        # (Expande el borde del tile 8 píxeles hacia afuera)
        border_left = t_img.crop((0, 0, 1, 32)).resize((8, 32))
        border_right = t_img.crop((31, 0, 32, 32)).resize((8, 32))
        border_top = t_img.crop((0, 0, 32, 1)).resize((32, 8))
        border_bottom = t_img.crop((0, 31, 32, 32)).resize((32, 8))
        
        sheet.paste(border_left, (gx, gy+8))
        sheet.paste(border_right, (gx+40, gy+8))
        sheet.paste(border_top, (gx+8, gy))
        sheet.paste(border_bottom, (gx+8, gy+40))
        
        # Esquinas
        c_tl = t_img.crop((0,0,1,1)).resize((8,8))
        c_tr = t_img.crop((31,0,32,1)).resize((8,8))
        c_bl = t_img.crop((0,31,1,32)).resize((8,8))
        c_br = t_img.crop((31,31,32,32)).resize((8,8))
        
        sheet.paste(c_tl, (gx, gy))
        sheet.paste(c_tr, (gx+40, gy))
        sheet.paste(c_bl, (gx, gy+40))
        sheet.paste(c_br, (gx+40, gy+40))

    sheet.save(hd_sheet)
    with open(f"{hd_dir}/manifest.json", 'w') as f:
        json.dump(manifest, f, indent=2)
        
    print(f"✨ Spritesheet HD 4K reensamblado con éxito en: {hd_sheet}")

if __name__ == "__main__":
    if len(sys.argv) < 3:
        print("Uso: python3 tile_tool.py [unpack|pack] 'Nombre_ROM'")
        sys.exit(1)
        
    action = sys.argv[1]
    rom = sys.argv[2]
    
    if action == "unpack":
        unpack(rom)
    elif action == "pack":
        pack(rom)
