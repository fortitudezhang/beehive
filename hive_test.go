package beehive

import (
	"encoding/binary"
	"fmt"
	"runtime"
	"strconv"
	"testing"
)

const (
	handlers int = 1
	msgs         = 2
)

var testHiveCh = make(chan interface{})

type MyMsg int

type testHiveHandler struct{}

func (h *testHiveHandler) Map(m Msg, c MapContext) MappedCells {
	v := int(m.Data().(MyMsg))
	return MappedCells{{"D", strconv.Itoa(v % handlers)}}
}

func intToBytes(i int) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(i))
	return buf
}

func bytesToInt(b []byte) int {
	return int(binary.LittleEndian.Uint64(b))
}

func (h *testHiveHandler) Rcv(m Msg, c RcvContext) error {
	hash := int(m.Data().(MyMsg)) % handlers
	d := c.State().Dict("D")
	k := strconv.Itoa(hash)
	v, err := d.Get(k)
	i := 1
	if err == nil {
		i += bytesToInt(v)
	}

	if err = d.Put(k, intToBytes(i)); err != nil {
		panic(fmt.Sprintf("Cannot change the key: %v", err))
	}

	id := c.ID() % uint64(handlers)
	if id != uint64(hash) {
		panic(fmt.Sprintf("Invalid message %v received in %v.", m, id))
	}

	if i == msgs/handlers {
		testHiveCh <- true
	}

	return nil
}

func runHiveTest(cfg HiveConfig, t *testing.T) {
	runtime.GOMAXPROCS(4)
	defer runtime.GOMAXPROCS(1)

	testHiveCh = make(chan interface{})
	defer func() {
		close(testHiveCh)
		testHiveCh = nil
	}()

	hive := NewHiveWithConfig(cfg)
	app := hive.NewApp("TestHiveApp")
	app.Handle(MyMsg(0), &testHiveHandler{})

	go hive.Start()

	for i := 1; i <= msgs; i++ {
		hive.Emit(MyMsg(i))
	}

	for i := 0; i < handlers; i++ {
		<-testHiveCh
	}

	if err := hive.Stop(); err != nil {
		t.Errorf("Cannot stop the hive %v", err)
	}
}

func TestHiveStart(t *testing.T) {
	cfg := DefaultCfg
	cfg.StatePath = "/tmp/bhtest"
	defer removeState(cfg)
	runHiveTest(DefaultCfg, t)
}

func TestHiveRestart(t *testing.T) {
	cfg := DefaultCfg
	cfg.StatePath = "/tmp/bhtest"
	defer removeState(cfg)
	runHiveTest(DefaultCfg, t)
	runHiveTest(DefaultCfg, t)
}

func TestCluster(t *testing.T) {
	cfg1 := DefaultCfg
	cfg1.StatePath = "/tmp/bhtest1"
	cfg1.Addr = "127.0.0.1:7767"
	defer removeState(cfg1)
	h1 := NewHiveWithConfig(cfg1)
	go h1.Start()
	waitTilStareted(h1)

	cfg2 := DefaultCfg
	cfg2.StatePath = "/tmp/bhtest2"
	cfg2.Addr = "127.0.0.1:7777"
	cfg2.PeerAddrs = []string{"127.0.0.1:7767"}
	defer removeState(cfg2)
	h2 := NewHiveWithConfig(cfg2)
	go h2.Start()

	cfg3 := DefaultCfg
	cfg3.StatePath = "/tmp/bhtest3"
	cfg3.Addr = "127.0.0.1:7787"
	cfg3.PeerAddrs = []string{"127.0.0.1:7767"}
	defer removeState(cfg3)
	h3 := NewHiveWithConfig(cfg3)
	go h3.Start()

	waitTilStareted(h2)
	waitTilStareted(h3)

	h3.Stop()
	h2.Stop()
	h1.Stop()
}