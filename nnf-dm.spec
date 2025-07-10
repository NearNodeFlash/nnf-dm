%global debug_package %{nil}

# Epochs are only required for TOSS4 due to existing 1.0-betaX
# packages which predate the existing tagged version numbers.
%if 0%{?toss} == 4
Epoch:		1
%endif

Name:		nnf-datamovement
Version:	0.1.19
Release:	1%{?dist}
Summary:	NNF Data Movement Daemon

Group:		System Environment/Daemons
License:	Apache-2.0
URL:		https://github.com/NearNodeFlash/nnf-dm
Source0:	nnf-dm-%{version}.tar.gz
BuildRequires:  json-c-devel
Requires:       json-c
BuildRequires:  libcurl-devel
Requires:       libcurl
ExclusiveArch:	x86_64

%description
This package provides the data movement server for Near Node Flash. This allows
Near Node Flash data movement through the data movement API (Rabbit).

Provides Near Node Flash Data Movement's libcopyoffload.

%prep
%setup -q -n nnf-dm-%{version}

%build
pushd daemons/lib-copy-offload
make libcopyoffload.a libcopyoffload.so tester-dynamic
popd

%install
install -D -m 755 daemons/lib-copy-offload/tester-dynamic %{buildroot}/%{_bindir}/nnf-dm
install -D -m 755 daemons/lib-copy-offload/libcopyoffload.a %{buildroot}/%{_libdir}/libcopyoffload.a
install -D -m 755 daemons/lib-copy-offload/libcopyoffload.so %{buildroot}/%{_libdir}/libcopyoffload.so
install -D -m 644 daemons/lib-copy-offload/copy-offload.h %{buildroot}/%{_includedir}/copy-offload.h
install -D -m 644 daemons/lib-copy-offload/copy-offload-status.h %{buildroot}/%{_includedir}/copy-offload-status.h

%files
%{_bindir}/nnf-dm
%{_libdir}/libcopyoffload.a
%{_libdir}/libcopyoffload.so

%package devel
Summary: NNF Data Movement devel package
Group: System Environment/Daemons
%if 0%{?toss} == 4
Requires: %{name} = %{epoch}:%{version}-%{release}
%else
Requires: %{name} = %{version}-%{release}
%endif

%description devel
Development package containing headers associated with Near Node Flash
Data Movement's libcopyoffload.

%files devel
%defattr(-,root,root)
%{_includedir}/copy-offload.h
%{_includedir}/copy-offload-status.h
