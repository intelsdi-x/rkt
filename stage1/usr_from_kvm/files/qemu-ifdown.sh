#!/bin/bash

stage1/rootfs/usr/bin/chmod +x stage1/rootfs/usr/bin/busybox

SED="stage1/rootfs/usr/bin/busybox sed"
BRCTL="stage1/rootfs/usr/bin/busybox brctl"
TUNCTL="stage1/rootfs/usr/bin/busybox tunctl"
IP="stage1/rootfs/usr/bin/ip"

NUM=`echo $1 | sed 's/[a-z]//g'`
BRNAME="br$NUM"
IFACE="eth$NUM"

$BRCTL delif $BRNAME $IFACE
$BRCTL delif $BRNAME $1
$IP link set $BRNAME down
$BRCTL delbr $BRNAME
