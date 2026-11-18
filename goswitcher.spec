Name:           goswitcher
Version:        1.2.0
Release:        1%{?dist}
Summary:        AI-powered keyboard layout switcher and typing assistant for Linux
License:        MIT
URL:            https://github.com/nikodim/goswitcher
Source0:        %{name}-%{version}.tar.gz

BuildRequires:  go >= 1.21
BuildRequires:  pkgconfig(ayatana-appindicator3-0.1)
BuildRequires:  pkgconfig(gtk+-3.0)
BuildRequires:  systemd-rpm-macros
BuildRequires:  udev

Requires:       libayatana-appindicator3-1
Requires:       wl-clipboard
Requires:       xclip
Requires:       udev

%description
GoSwitcher is a lightweight, low-level keyboard layout switcher and intelligent
typing assistant written in Go. Using evdev and uinput, it works natively on
both Wayland and X11 (KDE Plasma, GNOME), featuring a Naive Bayes language
classifier, QWERTY motor topography heatmap, DevOps syntax protection, and an
embedded Web UI dashboard.

%prep
%autosetup

%build
# Собираем с использованием vendored-зависимостей (требование изолированной RPM-сборки)
go build -mod=vendor -ldflags="-s -w" -trimpath -o goswitcher-bin .

%install
# 1. Директория приложения с предобученной моделью и словарями
install -d -m 0755 %{buildroot}%{_libexecdir}/%{name}
install -m 0755 goswitcher-bin %{buildroot}%{_libexecdir}/%{name}/goswitcher-bin
if [ -f model.json ]; then
    install -m 0644 model.json %{buildroot}%{_libexecdir}/%{name}/model.json
fi
if [ -f words_ru.txt ]; then
    install -m 0644 words_ru.txt %{buildroot}%{_libexecdir}/%{name}/words_ru.txt
fi
if [ -f words_en.txt ]; then
    install -m 0644 words_en.txt %{buildroot}%{_libexecdir}/%{name}/words_en.txt
fi

# 2. Wrapper в /usr/bin/goswitcher: инициализирует ~/.local/share/goswitcher
#    для хранения изменяемых settings.json, stats.json и heatmap.json от имени пользователя
install -d -m 0755 %{buildroot}%{_bindir}
cat << 'EOF' > %{buildroot}%{_bindir}/%{name}
#!/bin/sh
DATA_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/goswitcher"
mkdir -p "$DATA_DIR"

if [ ! -f "$DATA_DIR/model.json" ] && [ -f "%{_libexecdir}/%{name}/model.json" ]; then
    cp "%{_libexecdir}/%{name}/model.json" "$DATA_DIR/model.json"
fi
[ -f "%{_libexecdir}/%{name}/words_ru.txt" ] && ln -sf "%{_libexecdir}/%{name}/words_ru.txt" "$DATA_DIR/words_ru.txt"
[ -f "%{_libexecdir}/%{name}/words_en.txt" ] && ln -sf "%{_libexecdir}/%{name}/words_en.txt" "$DATA_DIR/words_en.txt"
ln -sf "%{_libexecdir}/%{name}/goswitcher-bin" "$DATA_DIR/goswitcher-bin"

exec "$DATA_DIR/goswitcher-bin" "$@"
EOF
chmod 0755 %{buildroot}%{_bindir}/%{name}

# 3. Правило udev для доступа группы input к /dev/uinput без прав root
install -d -m 0755 %{buildroot}%{_udevrulesdir}
cat << 'EOF' > %{buildroot}%{_udevrulesdir}/99-goswitcher.rules
KERNEL=="uinput", MODE="0660", GROUP="input", OPTIONS+="static_node=uinput"
EOF

# 4. Systemd user-сервис для фоновой работы и автоперезапуска
install -d -m 0755 %{buildroot}%{_userunitdir}
cat << 'EOF' > %{buildroot}%{_userunitdir}/goswitcher.service
[Unit]
Description=GoSwitcher AI Keyboard Layout Switcher
After=graphical-session.target

[Service]
Type=simple
ExecStart=%{_bindir}/%{name}
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
EOF

# 5. Ярлык .desktop для меню KDE Plasma
install -d -m 0755 %{buildroot}%{_datadir}/applications
cat << 'EOF' > %{buildroot}%{_datadir}/applications/goswitcher.desktop
[Desktop Entry]
Type=Application
Name=GoSwitcher
Comment=AI Keyboard Layout Switcher & Typing Assistant
Exec=%{_bindir}/%{name}
Icon=input-keyboard
Terminal=false
Categories=Utility;System;
EOF

%post
%udev_rules_update
echo "----------------------------------------------------------------------"
echo "GoSwitcher installed! Make sure your user is in the 'input' group:"
echo "  sudo usermod -aG input \$USER"
echo "Then re-login and enable the user service:"
echo "  systemctl --user enable --now goswitcher.service"
echo "----------------------------------------------------------------------"

%postun
%udev_rules_update

%files
%{_bindir}/%{name}
%dir %{_libexecdir}/%{name}
%{_libexecdir}/%{name}/goswitcher-bin
%attr(0644,root,root) %{_libexecdir}/%{name}/model.json
%attr(0644,root,root) %{_libexecdir}/%{name}/words_*.txt
%{_udevrulesdir}/99-goswitcher.rules
%{_userunitdir}/goswitcher.service
%{_datadir}/applications/goswitcher.desktop

%changelog
* Wed Nov 18 2026 NikoDim <nikodim@localhost> - 1.2.0-1
- Initial RPM package for openSUSE Tumbleweed with evdev/uinput and systemd user unit
