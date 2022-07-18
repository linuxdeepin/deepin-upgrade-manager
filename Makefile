PRJ=deepin-upgrade-manager
PROG_UPGRADER=${PRJ}
PROG_BOOTKIT=deepin-boot-kit
PROG_DBUS=org.deepin.AtomicUpgrade1
PREFIX=/usr
VAR=/var/lib
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
	@env GOPATH=${PWD}/${GOPATH_DIR}:${GOCODE}:${GOPATH} ls -al ${PWD}/${GOPATH_DIR}/src/deepin-upgrade-manager/cmd/deepin-boot-kit/*
	@env GOPATH=${PWD}/${GOPATH_DIR}:${GOCODE}:${GOPATH} go build -o ${PWD}/${PROG_UPGRADER} ${PRJ}/cmd/${PROG_UPGRADER}
	@env GOPATH=${PWD}/${GOPATH_DIR}:${GOCODE}:${GOPATH} go build -o ${PWD}/${PROG_BOOTKIT} ${PRJ}/cmd/${PROG_BOOTKIT}

install-upgrader:
#ifeq ($(ARCH),sw_64)
#	mkdir -p ${DESTDIR}/etc/grub.d/sw64
#	cp -f ${PWD}/cmd/grub.d/sw/15_deepin-upgrade-manager ${DESTDIR}/etc/grub.d/sw64
#endif
	@mkdir -p ${DESTDIR}/etc/${PROG_UPGRADER}/
	@cp -f ${PWD}/configs/upgrader/config.simple.json  ${DESTDIR}/etc/${PROG_UPGRADER}/config.json

	@mkdir -p ${DESTDIR}${VAR}/${PROG_BOOTKIT}/config
	@cp -f ${PWD}/configs/upgrader/tool/atomic.json  ${DESTDIR}${VAR}/${PROG_BOOTKIT}/config/atomic.json

	@mkdir -p ${DESTDIR}${VAR}/${PROG_UPGRADER}/scripts
	@cp -f ${PWD}/cmd/initramfs-scripts/${PROG_UPGRADER} ${DESTDIR}${VAR}/${PROG_UPGRADER}/scripts/

	@mkdir -p ${DESTDIR}${PREFIX}/share/dbus-1/system.d/
	@cp -f ${PWD}/configs/dbus/${PROG_DBUS}.conf  ${DESTDIR}${PREFIX}/share/dbus-1/system.d/

	@mkdir -p ${DESTDIR}${PREFIX}/share/dbus-1/system-services/
	@cp -f ${PWD}/configs/dbus/${PROG_DBUS}.service  ${DESTDIR}${PREFIX}/share/dbus-1/system-services/

	@mkdir -p ${DESTDIR}${PREFIX}/sbin
	@cp -f ${PWD}/${PROG_UPGRADER} ${DESTDIR}${PREFIX}/sbin

	@mkdir -p ${DESTDIR}${PREFIX}/share/initramfs-tools/hooks
	@cp -f ${PWD}/cmd/initramfs-hook/${PROG_UPGRADER} ${DESTDIR}${PREFIX}/share/initramfs-tools/hooks/
	@cp -f ${PWD}/cmd/initramfs-hook/ostree ${DESTDIR}${PREFIX}/share/initramfs-tools/hooks/

	@mkdir -p ${DESTDIR}${PREFIX}/share/initramfs-tools/scripts/init-bottom
	@cp -f ${PWD}/cmd/initramfs-scripts/${PROG_UPGRADER} ${DESTDIR}${PREFIX}/share/initramfs-tools/scripts/init-bottom/

install-bootkit:
	@mkdir -p ${DESTDIR}/usr/share/${PROG_BOOTKIT}/
	@cp -f ${PWD}/configs/bootkit/config.simple.json  ${DESTDIR}/usr/share/${PROG_BOOTKIT}/config.json

	@mkdir -p ${DESTDIR}${PREFIX}/sbin
	@cp -f ${PWD}/${PROG_BOOTKIT} ${DESTDIR}${PREFIX}/sbin/

	@mkdir -p ${DESTDIR}/etc/grub.d ${DESTDIR}/etc/default/grub.d
	@cp -f ${PWD}/cmd/grub.d/15_deepin-boot-kit ${DESTDIR}/etc/grub.d

	@mkdir -p ${DESTDIR}${PREFIX}/share/initramfs-tools/hooks
	@cp -f ${PWD}/cmd/initramfs-hook/${PROG_BOOTKIT} ${DESTDIR}${PREFIX}/share/initramfs-tools/hooks/

	@mkdir -p ${DESTDIR}${PREFIX}/share/initramfs-tools/scripts/init-bottom
	@cp -f ${PWD}/cmd/initramfs-scripts/${PROG_BOOTKIT} ${DESTDIR}${PREFIX}/share/initramfs-tools/scripts/init-bottom/

install: install-upgrader install-bootkit

uninstall-upgrader:
#ifeq ($(ARCH),sw_64)
#	rm -f ${DESTDIR}/etc/grub.d/sw64/15_deepin-upgrade-manager
#endif
	@rm -f ${DESTDIR}${PREFIX}/sbin/${PROG_UPGRADER}
	@rm -f ${DESTDIR}${PREFIX}/share/dbus-1/system.d/${PROG_DBUS}.conf
	@rm -f ${DESTDIR}${PREFIX}/share/dbus-1/system-services/${PROG_DBUS}.service
	@rm -f ${DESTDIR}/etc/${PROG_UPGRADER}/config.json
	@rm -f ${DESTDIR}${PREFIX}/share/initramfs-tools/hooks/ostree
	@rm -f ${DESTDIR}${PREFIX}/share/initramfs-tools/hooks/${PROG_UPGRADER}
	@rm -f ${DESTDIR}${VAR}/${PROG_UPGRADER}/scripts/${PROG_UPGRADER}
	@rm -f ${DESTDIR}${VAR}/${PROG_BOOTKIT}/config/atomic.json

uninstall-bootkit:
	@rm -f ${DESTDIR}${PREFIX}/sbin/${PROG_BOOTKIT}
	@rm -f ${DESTDIR}/usr/share/${PROG_BOOTKIT}/config.json
	@rm -f ${DESTDIR}/etc/grub.d/15_deepin-boot-kit
	@rm -f ${DESTDIR}${PREFIX}/share/initramfs-tools/hooks/${PROG_BOOTKIT}
	@rm -f ${DESTDIR}${PREFIX}/share/initramfs-tools/scripts/init-bottom/${PROG_BOOTKIT}

uninstall: uninstall-upgrader uninstall-bootkit

clean:
	@rm -rf ${GOPATH_DIR}
	@rm -rf ${PWD}/${PROG_UPGRADER}
	@rm -rf ${PWD}/${PROG_BOOTKIT}

rebuild: clean build
