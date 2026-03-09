Name:           dub
Version:        0.1.0
Release:        1%{?dist}
Summary:        Batch File Renamer
License:        MIT
URL:            https://github.com/omegaatt36/dub
AutoReqProv:    no

%description
A Batch File Renamer application built with Go, HTMX, and Wails.

%prep
# No prep needed as we are using pre-built binaries

%build
# No build needed

%install
mkdir -p %{buildroot}/usr/bin
mkdir -p %{buildroot}/usr/share/applications
mkdir -p %{buildroot}/usr/share/icons/hicolor/512x512/apps

cp %{project_root}/build/bin/dub %{buildroot}/usr/bin/
cp %{project_root}/build/linux/dub.desktop %{buildroot}/usr/share/applications/
cp %{project_root}/build/linux/dub.png %{buildroot}/usr/share/icons/hicolor/512x512/apps/

%files
%defattr(-,root,root,-)
%attr(755,root,root) /usr/bin/dub
/usr/share/applications/dub.desktop
/usr/share/icons/hicolor/512x512/apps/dub.png

%changelog
* Mon Mar 09 2026 raiven_kao <raiven.kao@gmail.com> - 0.1.0-1
- Initial RPM package for openSUSE Tumbleweed
