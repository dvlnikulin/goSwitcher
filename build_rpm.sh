#!/usr/bin/env bash
set -euo pipefail

NAME="goswitcher"
VERSION="1.2.0"
TARBALL="${NAME}-${VERSION}.tar.gz"
BUILD_ROOT="$(mktemp -d)"

trap 'rm -rf "$BUILD_ROOT"' EXIT

echo "[1/4] Проверка зависимостей для сборки RPM..."
sudo zypper install -y rpm-build go libayatana-appindicator3-devel gtk3-devel

echo "[2/4] Подготовка vendor-модулей и архива исходников..."
go mod tidy
go mod vendor

# Если model.json еще не сгенерирован, собираем его перед упаковкой
if [ ! -f "model.json" ] && [ -f "words_ru.txt" ] && [ -f "words_en.txt" ]; then
    echo "Генерация model.json..."
    go run . -learn
fi

mkdir -p "${BUILD_ROOT}/${NAME}-${VERSION}"
cp -r main.go ai.go hook_linux.go platform_linux.go go.mod go.sum vendor web icon.ico "${BUILD_ROOT}/${NAME}-${VERSION}/"
[ -f "model.json" ] && cp model.json "${BUILD_ROOT}/${NAME}-${VERSION}/"
[ -f "words_ru.txt" ] && cp words_ru.txt "${BUILD_ROOT}/${NAME}-${VERSION}/"
[ -f "words_en.txt" ] && cp words_en.txt "${BUILD_ROOT}/${NAME}-${VERSION}/"

mkdir -p ~/rpmbuild/{BUILD,RPMS,SOURCES,SPECS,SRPMS}
tar -czf ~/rpmbuild/SOURCES/"${TARBALL}" -C "${BUILD_ROOT}" "${NAME}-${VERSION}"
cp goswitcher.spec ~/rpmbuild/SPECS/

echo "[3/4] Запуск rpmbuild..."
rpmbuild -ba ~/rpmbuild/SPECS/goswitcher.spec

echo "[4/4] Готово! Собранный пакет:"
find ~/rpmbuild/RPMS/ -name "${NAME}-${VERSION}*.rpm"
