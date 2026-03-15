#!/bin/bash
# Binance bot + API süreçlerini tamamen durdurur (sagı solu kontrol et).

set -e
cd "$(dirname "$0")/.."

echo "Durduruluyor: api.bin, go run ./cmd/bot, arka planda kalan bot process'leri..."
pkill -f "api.bin" 2>/dev/null || true
pkill -f "cmd/bot"  2>/dev/null || true
# go run ile açılan bot binary'si /var/.../go-build.../exe/bot veya .../Caches/go-build.../bot olarak çalışır
pkill -f "exe/bot" 2>/dev/null || true
pkill -f "Caches/go-build.*/bot" 2>/dev/null || true
sleep 1
pkill -9 -f "api.bin" 2>/dev/null || true
pkill -9 -f "cmd/bot" 2>/dev/null || true
pkill -9 -f "exe/bot" 2>/dev/null || true
pkill -9 -f "Caches/go-build.*/bot" 2>/dev/null || true
sleep 1

if pgrep -f "api.bin" >/dev/null 2>&1 || pgrep -f "cmd/bot" >/dev/null 2>&1 || pgrep -f "exe/bot" >/dev/null 2>&1; then
  echo "Uyarı: Bazı süreçler hâlâ çalışıyor. Kontrol: ps -ef | grep -E 'api.bin|cmd/bot|exe/bot'"
  exit 1
fi

echo "OK: Bot ve API durduruldu."
if lsof -i :8080 >/dev/null 2>&1; then
  echo "8080 hâlâ kullanımda; lsof -i :8080 ile kontrol et."
else
  echo "8080 portu boş."
fi
