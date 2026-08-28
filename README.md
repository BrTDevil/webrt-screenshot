# WebRT scrennshots

Captură de ecran pentru un site web, direct în WebP, în linie de comandă. Folosește Chrome headless (via [chromedp](https://github.com/chromedp/chromedp)) și emulează presetări de device (desktop / tabletă / mobil), cu suport pentru captură doar a zonei vizibile sau a paginii complete.

## Cerințe

- Go 1.22+ (pentru compilare)
- Google Chrome sau Chromium instalat local (`google-chrome`, `google-chrome-stable`, `chromium-browser` sau `chromium` — se caută în această ordine în `PATH`)

## Compilare

```bash
cd scrennshots
go build -o scrennshots .
```

## Utilizare de bază

```bash
# desktop, doar zona vizibilă (implicit)
./scrennshots -url site.ro

# pagină completă (tot scroll-ul)
./scrennshots -url site.ro -full

# mobil, pagină completă
./scrennshots -url site.ro -device mobile -full

# toate cele 3 device-uri dintr-o dată
./scrennshots -url site.ro -all -full
```

## Opțiuni

| Flag | Implicit | Descriere |
|---|---|---|
| `-url` | — | URL-ul paginii de capturat (obligatoriu; `https://` se adaugă automat dacă lipsește) |
| `-device` | `desktop` | Preset: `desktop` (1920×1080), `tablet` (768×1024 @2x) sau `mobile` (390×844 @3x) |
| `-all` | `false` | Captură pentru toate cele 3 device-uri dintr-o dată; ignoră `-device`, `-width`, `-height`, `-out` |
| `-full` | `false` | Captură pe toată înălțimea paginii; implicit doar zona vizibilă (viewport) |
| `-width` | — | Lățime custom în px, suprascrie preset-ul de device (ignorat cu `-all`) |
| `-height` | — | Înălțime custom în px, suprascrie preset-ul de device (ignorat cu `-all`) |
| `-q` | `85` | Calitate WebP (0-100) |
| `-wait` | `500ms` | Așteptare suplimentară după încărcare/scroll, pentru animații și conținut lazy |
| `-timeout` | `30s` | Timeout total pentru încărcarea paginii |
| `-out` | — | Numele fișierului de ieșire (implicit generat din URL + device; ignorat cu `-all`) |
| `-dir` | `.` | Directorul unde se salvează captura |

Fără `-out`, fișierele se numesc automat `<host>_<cale>_<device>_<viewport\|full>.webp`, de exemplu `site_ro_desktop_full.webp`.

## Cum funcționează captura de pagină completă (`-full`)

Pe site-uri React/Vue cu animații la scroll (AOS, framer-motion `whileInView` etc.) sau imagini `loading="lazy"`, un simplu screenshot pe toată înălțimea paginii lasă cardurile de mai jos de primul ecran goale, fiindcă acel conținut nu a fost niciodată "văzut" de browser.

Pentru `-full`, unealta:

1. face un scroll rapid de la sus până jos (declanșează imaginile lazy și listenerii de `scroll`);
2. redimensionează manual viewportul la înălțimea totală a paginii, astfel încât tot conținutul devine vizibil simultan — asta declanșează o singură dată, definitiv, orice animație bazată pe `IntersectionObserver` (multe din ele se resetează dacă elementul iese din viewport, ceea ce un simplu "scroll jos + revino sus" ar fi provocat);
3. așteaptă explicit (max. 5s) ca toate imaginile `<img>` să termine de încărcat înainte de a face captura efectivă.

Dacă pagina e foarte lungă și device-ul are un factor de scalare mare (ex. `mobile` @3x), imaginea rezultată poate depăși limita de 16383px/latură a formatului WebP — în acest caz e redimensionată automat, păstrând proporția, ca encodarea să nu eșueze.

## Exemple

```bash
# captură completă pentru toate device-urile, salvată într-un folder dedicat
./scrennshots -url webrt.eu -all -full -dir webrt.eu

# pagină grea, cu mai mult timp de așteptare pentru animații/imagini
./scrennshots -url site.ro -full -wait 2s -timeout 60s

# doar tabletă, calitate mai mare, nume de fișier custom
./scrennshots -url site.ro -device tablet -q 95 -out preview-tableta
```

## Instalare globală (`webrt-screenshot` / `webrt:screenshot`, în orice terminal)

Binarul e copiat în `/opt/webrt-tools/bin` (sub numele `screenshot`), iar în `/usr/local/bin` (care e în `PATH` pe orice distribuție Linux, indiferent de shell) se creează symlink-uri sub ambele nume, cu liniuță și cu două puncte:

```bash
cd scrennshots
go build -o scrennshots .

sudo mkdir -p /opt/webrt-tools/bin
sudo install -m 755 scrennshots /opt/webrt-tools/bin/screenshot

sudo ln -sf /opt/webrt-tools/bin/screenshot /usr/local/bin/webrt-screenshot
sudo ln -sf /opt/webrt-tools/bin/screenshot /usr/local/bin/webrt:screenshot
```

După asta, din orice director, în orice terminal:

```bash
webrt-screenshot -url site.ro -all -full
# sau
webrt:screenshot -url site.ro -all -full
```

Dezinstalare:

```bash
sudo rm -f /usr/local/bin/webrt-screenshot /usr/local/bin/webrt:screenshot /opt/webrt-tools/bin/screenshot
```
