%undefine _missing_build_ids_terminate_build
%global debug_package %{nil}

Name: nnf-datamovement
Version: 1.0
Release: 1%{?dist}
Summary: Near Node Flash data movement daemon

Group: 1
License: Apache-2.0
URL: https://github.com/NearNodeFlash/nnf-dm
Source0: %{name}-%{version}.tar.gz

BuildRequires:	golang

%description
This package provides the data movement server for Near Node Flash. This allows
Near Node Flash data movement through the data movement API.

%prep
%setup -q

%build
RPM_VERSION=$(cat .rpmversion) make build-daemon

%install
mkdir -p %{buildroot}/usr/bin/
install -m 755 bin/nnf-dm %{buildroot}/usr/bin/nnf-dm

%files
/usr/bin/nnf-dm
