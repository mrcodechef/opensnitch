package ebpf

import (
	"fmt"
	"unsafe"

	"github.com/evilsocket/opensnitch/daemon/core"
	"github.com/evilsocket/opensnitch/daemon/log"
)

func mountDebugFS() error {
	debugfsPath := "/sys/kernel/debug/"
	kprobesPath := fmt.Sprint(debugfsPath, "tracing/kprobe_events")
	if core.Exists(kprobesPath) == false {
		if _, err := core.Exec("mount", []string{"-t", "debugfs", "none", debugfsPath}); err != nil {
			log.Warning("eBPF debugfs error: %s", err)
			return err
		}
	}

	return nil
}

func deleteEbpfEntry(proto string, key unsafe.Pointer) bool {
	if err := m.DeleteElement(ebpfMaps[proto].bpfmap, key); err != nil {
		return false
	}
	return true
}

func getItems(proto string, isIPv6 bool) (items uint) {
	isDup := make(map[string]uint8)
	var lookupKey []byte
	var nextKey []byte

	if !isIPv6 {
		lookupKey = make([]byte, 12)
		nextKey = make([]byte, 12)
	} else {
		lookupKey = make([]byte, 36)
		nextKey = make([]byte, 36)
	}
	var value networkEventT
	firstrun := true

	for {
		ok, err := m.LookupNextElement(ebpfMaps[proto].bpfmap, unsafe.Pointer(&lookupKey[0]),
			unsafe.Pointer(&nextKey[0]), unsafe.Pointer(&value))
		if !ok || err != nil { //reached end of map
			log.Debug("[ebpf] %s map: %d active items", proto, items)
			return
		}
		if firstrun {
			// on first run lookupKey is a dummy, nothing to delete
			firstrun = false
			copy(lookupKey, nextKey)
			continue
		}
		if counter, duped := isDup[string(lookupKey)]; duped && counter > 1 {
			deleteEbpfEntry(proto, unsafe.Pointer(&lookupKey[0]))
			continue
		}
		isDup[string(lookupKey)]++
		copy(lookupKey, nextKey)
		items++
	}

	return items
}

// deleteOldItems deletes maps' elements in order to keep them below maximum capacity.
// If ebpf maps are full they don't allow any more insertions, ending up lossing events.
func deleteOldItems(proto string, isIPv6 bool, maxToDelete uint) (deleted uint) {
	isDup := make(map[string]uint8)
	var lookupKey []byte
	var nextKey []byte
	if !isIPv6 {
		lookupKey = make([]byte, 12)
		nextKey = make([]byte, 12)
	} else {
		lookupKey = make([]byte, 36)
		nextKey = make([]byte, 36)
	}
	var value networkEventT
	firstrun := true
	i := uint(0)

	for {
		i++
		if i > maxToDelete {
			return
		}
		ok, err := m.LookupNextElement(ebpfMaps[proto].bpfmap, unsafe.Pointer(&lookupKey[0]),
			unsafe.Pointer(&nextKey[0]), unsafe.Pointer(&value))
		if !ok || err != nil { //reached end of map
			return
		}
		if _, duped := isDup[string(lookupKey)]; duped {
			if deleteEbpfEntry(proto, unsafe.Pointer(&lookupKey[0])) {
				deleted++
				copy(lookupKey, nextKey)
				continue
			}
			return
		}

		if firstrun {
			// on first run lookupKey is a dummy, nothing to delete
			firstrun = false
			copy(lookupKey, nextKey)
			continue
		}

		if !deleteEbpfEntry(proto, unsafe.Pointer(&lookupKey[0])) {
			return
		}
		deleted++
		isDup[string(lookupKey)]++
		copy(lookupKey, nextKey)
	}

	return
}
