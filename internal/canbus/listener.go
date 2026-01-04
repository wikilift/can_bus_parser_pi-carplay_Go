package canbus

import (
	"log"
	"math"
	"runtime"
	"sync"
	"time"

	"can-service/internal/models"

	"github.com/brutella/can"
)

type Reading struct {
	Name  string
	Value float32
}

type CanStatus struct {
	OK  bool
	Err string
}

type CanListener struct {
	interfaceName string
	outChan       chan Reading
	stopChan      chan struct{}
	isRunning     bool
	mu            sync.Mutex

	statusMu sync.RWMutex
	status   CanStatus

	speedMu         sync.Mutex
	speedHave       bool
	speedLastCnt    uint16
	speedLastAt     time.Time
	speedLastKmh    float32
	speedLastUpdate time.Time
}

func NewCanListener(iface string) *CanListener {
	return &CanListener{
		interfaceName: iface,
		outChan:       make(chan Reading, 400),
		stopChan:      make(chan struct{}),
		status:        CanStatus{OK: false, Err: "not started"},
	}
}

func (c *CanListener) Readings() <-chan Reading { return c.outChan }

func (c *CanListener) Status() CanStatus {
	c.statusMu.RLock()
	defer c.statusMu.RUnlock()
	return c.status
}

func (c *CanListener) setStatus(ok bool, err string) {
	c.statusMu.Lock()
	c.status.OK = ok
	c.status.Err = err
	c.statusMu.Unlock()
}

func (c *CanListener) Start(config models.CarConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isRunning {
		close(c.stopChan)
		c.stopChan = make(chan struct{})
		time.Sleep(50 * time.Millisecond)
	}

	c.speedMu.Lock()
	c.speedHave = false
	c.speedLastCnt = 0
	c.speedLastAt = time.Time{}
	c.speedLastKmh = 0
	c.speedLastUpdate = time.Time{}
	c.speedMu.Unlock()

	c.isRunning = true

	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		go c.runSimulation(config)
		return
	}

	go c.runRealHardwareOrFallback(config)
}

func (c *CanListener) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isRunning {
		close(c.stopChan)
		c.stopChan = make(chan struct{})
		c.isRunning = false
		c.setStatus(false, "stopped")
	}
}

func (c *CanListener) runRealHardwareOrFallback(config models.CarConfig) {
	log.Printf("opening %s model=%s", c.interfaceName, config.Name)

	bus, err := can.NewBusForInterfaceWithName(c.interfaceName)
	if err != nil {
		c.setStatus(false, err.Error())
		log.Printf("can unavailable: %v", err)
		go c.runSimulation(config)
		return
	}

	c.setStatus(true, "")

	bus.SubscribeFunc(func(frame can.Frame) {
		select {
		case <-c.stopChan:
			return
		default:
		}

		sig, ok := config.Signals[frame.ID]
		if !ok {
			return
		}

		m, ok := sig.Parse(frame.Data[:])
		if !ok || len(m) == 0 {
			return
		}

		if v, has := m["_speedCounter"]; has {
			c.updateSpeedFromCounter(uint16(v))
			return
		}

		for k, v := range m {
			c.emit(k, v)
		}
	})

	go func() {
		if err := bus.ConnectAndPublish(); err != nil {
			c.setStatus(false, err.Error())
			log.Printf("can bus error: %v", err)
		}
	}()

	<-c.stopChan
	bus.Disconnect()
	c.setStatus(false, "disconnected")
}

func (c *CanListener) updateSpeedFromCounter(cnt uint16) {
	now := time.Now()

	c.speedMu.Lock()
	defer c.speedMu.Unlock()

	if !c.speedHave {
		c.speedHave = true
		c.speedLastCnt = cnt
		c.speedLastAt = now
		return
	}

	dt := now.Sub(c.speedLastAt)
	ms := dt.Milliseconds()
	if ms < 50 || ms > 2000 {
		c.speedLastCnt = cnt
		c.speedLastAt = now
		return
	}

	delta := uint16(cnt - c.speedLastCnt)
	steps := float64(ms) / 50.0
	if steps <= 0 {
		c.speedLastCnt = cnt
		c.speedLastAt = now
		return
	}

	mph := float64(delta) / steps
	if mph < 0 {
		mph = 0
	}

	kmh := float32(mph * 1.609344)

	c.speedLastCnt = cnt
	c.speedLastAt = now

	if c.speedLastUpdate.IsZero() || now.Sub(c.speedLastUpdate) >= 100*time.Millisecond || math.Abs(float64(kmh-c.speedLastKmh)) >= 0.2 {
		c.speedLastKmh = kmh
		c.speedLastUpdate = now
		c.emit("speedKmh", kmh)
	}
}

func (c *CanListener) runSimulation(config models.CarConfig) {
	c.setStatus(false, "simulated")
	log.Println("simulation on")

	period := 50 * time.Millisecond
	redline := float64(7000)
	idle := float64(750)
	baseTemp := float64(86)
	fuelStart := float64(42)
	fuelReset := float64(45)
	fuelMin := float64(8)

	if config.Sim.PeriodMs > 0 {
		period = time.Duration(config.Sim.PeriodMs) * time.Millisecond
	}
	if config.Sim.RedlineRpm > 0 {
		redline = float64(config.Sim.RedlineRpm)
	}
	if config.Sim.IdleRpm > 0 {
		idle = float64(config.Sim.IdleRpm)
	}
	if config.Sim.BaseTempC > 0 {
		baseTemp = float64(config.Sim.BaseTempC)
	}
	if config.Sim.FuelStartL > 0 {
		fuelStart = float64(config.Sim.FuelStartL)
	}
	if config.Sim.FuelResetL > 0 {
		fuelReset = float64(config.Sim.FuelResetL)
	}
	if config.Sim.FuelMinL > 0 {
		fuelMin = float64(config.Sim.FuelMinL)
	}

	ticker := time.NewTicker(period)
	defer ticker.Stop()

	start := time.Now()
	var speed float64
	var rpm float64
	var temp float64 = baseTemp
	var fuel float64 = fuelStart
	var cons float64 = 7.2

	for {
		select {
		case <-c.stopChan:
			log.Println("simulation off")
			return
		case <-ticker.C:
			t := time.Since(start).Seconds()

			speed = 70 + 28*math.Sin(t*0.32) + 6*math.Sin(t*1.2)
			if speed < 0 {
				speed = 0
			}

			rpm = idle + speed*32 + 220*math.Sin(t*0.9)
			if rpm < idle {
				rpm = idle
			}
			if rpm > redline {
				rpm = redline
			}

			temp = baseTemp + 5*math.Sin(t*0.10)
			cons = 6.2 + 2.0*math.Abs(math.Sin(t*0.28)) + (speed/140.0)*3.0

			fuel = fuel - 0.0009
			if fuel < fuelMin {
				fuel = fuelReset
			}

			rangeKm := fuel * 12.5

			c.emit("speedKmh", float32(speed))
			c.emit("rpm", float32(rpm))
			c.emit("engineTempC", float32(temp))
			c.emit("instantConsumption", float32(cons))
			c.emit("fuelLiters", float32(fuel))
			c.emit("rangeKm", float32(rangeKm))
		}
	}
}

func (c *CanListener) emit(name string, v float32) {
	select {
	case c.outChan <- Reading{Name: name, Value: v}:
	default:
	}
}
