PRJ=deepin-upgrade-manager
PROG_UPGRADER=${PRJ}
PROG_DBUS=org.deepin.AtomicUpgrade
PREFIX=/usr
PWD=$(shell pwd)
GOCODE=/usr/share/gocode
GOPATH_DIR=gopath
CURRENT_DIR=$(notdir $(shell pwd))
ARCH=$(shell arch)
export GO111MODULE=off

all: build

prepare:
	@if [ ! -d ${GOPATH_DIR}/src/${PRJ} ]; then \
		mkdir -p ${GOPATH_DIR}/src/${PRJ}; \
		ln -sf ${PWD}/pkg ${GOPATH_DIR}/src/${PRJ}; \
		ln -sf ${PWD}/cmd ${GOPATH_DIR}/src/${PRJ}/; \
	fi

$(info, $(GOPATH))
$(warning, $(GOPATH))
$(error, $(GOPATH))

build: prepare
	@env GOPATH=${PWD}/${GOPATH_DIR}:${GOCODE}:${GOPATH} ls -al ${PWD}/${GOPATH_DIR}/src/deepin-upgrade-manager/*
	@env GOPATH=${PWD}/${GOPATH_DIR}:${GOCODE}:${GOPATH} ls -al ${PWD}/${GOPATH_DIR}/src/deepin-upgrade-manager/cmd/deepin-upgrade-manager/*
	@env GOPATH=${PWD}/${GOPATH_DIR}:${GOCODE}:${GOPATH} go build -o ${PWD}/${PROG_UPGRADER} ${PRJ}/cmd/${PROG_UPGRADER}

install:
ifeq ($(ARCH),sw_64)
	mkdir -p ${DESTDIR}/etc/grub.d/sw64
	cp -f ${PWD}/cmd/grub.d/sw/15_deepin-upgrade-manager ${DESTDIR}/etc/grub.d/sw64
endif
	@mkdir -p ${DESTDIR}/etc/${PRJ}/
	@cp -f ${PWD}/configs/config.simple.json  ${DESTDIR}/etc/${PRJ}/config.json
	@mkdir -p ${DESTDIR}${PREFIX}/share/dbus-1/system.d/
	@mkdir -p ${DESTDIR}${PREFIX}/share/dbus-1/system-services/
	@cp -f ${PWD}/configs/dbus/${PROG_DBUS}.conf  ${DESTDIR}${PREFIX}/share/dbus-1/system.d/
	@cp -f ${PWD}/configs/dbus/${PROG_DBUS}.service  ${DESTDIR}${PREFIX}/share/dbus-1/system-services/
	@mkdir -p ${DESTDIR}${PREFIX}/sbin
	@cp -f ${PWD}/${PROG_UPGRADER} ${DESTDIR}${PREFIX}/sbin/
	@mkdir -p ${DESTDIR}/etc/grub.d ${DESTDIR}/etc/default/grub.d
	@cp -f ${PWD}/cmd/grub.d/10_deepin-upgrade-manager.cfg ${DESTDIR}/etc/default/grub.d
	@cp -f ${PWD}/cmd/grub.d/15_deepin-upgrade-manager ${DESTDIR}/etc/grub.d
	@mkdir -p ${DESTDIR}${PREFIX}/share/initramfs-tools/hooks
	@cp -f ${PWD}/cmd/initramfs-hook/* ${DESTDIR}${PREFIX}/share/initramfs-tools/hooks/
	@mkdir -p ${DESTDIR}${PREFIX}/share/initramfs-tools/scripts/init-bottom
	@cp -f ${PWD}/cmd/initramfs-scripts/* ${DESTDIR}${PREFIX}/share/initramfs-tools/scripts/init-bottom/
	@mkdir -p ${DESTDIR}${PREFIX}/share/${PRJ}
	@cp -rf ${PWD}/scripts/apt.conf.d ${DESTDIR}${PREFIX}/share/${PRJ}/
	@cp -rf ${PWD}/scripts/dpkg.cfg.d ${DESTDIR}${PREFIX}/share/${PRJ}/

uninstall:
ifeq ($(ARCH),sw_64)
	rm -f ${DESTDIR}/etc/grub.d/sw64/15_deepin-upgrade-manager
endif
	@rm -f ${DESTDIR}${PREFIX}/sbin/${PROG_UPGRADER}
	@rm -f ${DESTDIR}${PREFIX}/share/dbus-1/system.d/${PROG_DBUS}.conf
	@rm -f ${DESTDIR}${PREFIX}/share/dbus-1/system-services/${PROG_DBUS}.service
	@rm -f ${DESTDIR}/etc/${PRJ}/config.json
	@rm -f ${DESTDIR}/etc/default/grub.d/10_deepin-upgrade-manager.cfg
	@rm -f ${DESTDIR}/etc/grub.d/15_deepin-upgrade-manager
	@rm -f ${DESTDIR}${PREFIX}/share/initramfs-tools/hooks/ostree
	@rm -f ${DESTDIR}${PREFIX}/share/initramfs-tools/hooks/${PROG_UPGRADER}
	@rm -f ${DESTDIR}${PREFIX}/share/initramfs-tools/scripts/init-bottom/${PROG_UPGRADER}
	@rm -rf ${DESTDIR}${PREFIX}/share/${PRJ}

clean:
	@rm -rf ${GOPATH_DIR}
	@rm -rf ${PWD}/${PROG_UPGRADER}

rebuild: clean build
