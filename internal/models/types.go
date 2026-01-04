package models

//TODO: separate this data by cars reversed enginered, evenmore will be interesting a sqlite or similar database and prepare the binary for remote updatable
import "encoding/binary"

type ValueParser func(data []byte) (map[string]float32, bool)

type SignalInfo struct {
	Parse ValueParser
}

type SimConfig struct {
	PeriodMs   int
	RedlineRpm int
	IdleRpm    int
	BaseTempC  int
	FuelStartL float32
	FuelResetL float32
	FuelMinL   float32
}

type CarConfig struct {
	Name    string
	Signals map[uint32]SignalInfo
	Sim     SimConfig
}

func u16le(b []byte, off int) (uint16, bool) {
	if len(b) < off+2 {
		return 0, false
	}
	return binary.LittleEndian.Uint16(b[off : off+2]), true
}

func u16be(b []byte, off int) (uint16, bool) {
	if len(b) < off+2 {
		return 0, false
	}
	return binary.BigEndian.Uint16(b[off : off+2]), true
}

func u24le(b []byte, off int) (uint32, bool) {
	if len(b) < off+3 {
		return 0, false
	}
	return uint32(b[off]) | uint32(b[off+1])<<8 | uint32(b[off+2])<<16, true
}

func DefaultSim() SimConfig {
	return SimConfig{
		PeriodMs:   50,
		RedlineRpm: 7000,
		IdleRpm:    750,
		BaseTempC:  86,
		FuelStartL: 42,
		FuelResetL: 45,
		FuelMinL:   8,
	}
}

func BMW_E87_Config() CarConfig {
	return CarConfig{
		Name: "BMW E87",
		Sim:  DefaultSim(),
		Signals: map[uint32]SignalInfo{
			0x1A6: {
				Parse: func(d []byte) (map[string]float32, bool) {
					c0, ok := u16le(d, 0)
					if !ok {
						return nil, false
					}
					return map[string]float32{"_speedCounter": float32(c0)}, true
				},
			},
			0x0AA: {
				Parse: func(d []byte) (map[string]float32, bool) {
					if len(d) < 8 {
						return nil, false
					}

					rawRpm := uint16(d[5])<<8 | uint16(d[4])
					rpm := float32(rawRpm) / 4.0

					rawThr := uint16(d[3])<<8 | uint16(d[2])
					throttleRaw := float32(rawThr)

					var throttlePct float32
					if rawThr <= 255 {
						throttlePct = 0
					} else {
						const minV = 255.0
						const maxV = 65064.0
						x := (float32(rawThr) - minV) / float32(maxV-minV)
						if x < 0 {
							x = 0
						}
						if x > 1 {
							x = 1
						}
						throttlePct = x * 100
					}

					return map[string]float32{
						"rpm":         rpm,
						"throttleRaw": throttleRaw,
						"throttlePct": throttlePct,
						"accelByte7":  float32(d[7]),
					}, true
				},
			},
			0x1D0: {
				Parse: func(d []byte) (map[string]float32, bool) {
					if len(d) < 1 {
						return nil, false
					}
					temp := float32(int(d[0]) - 48)
					return map[string]float32{"engineTempC": temp}, true
				},
			},
			0x349: {
				Parse: func(d []byte) (map[string]float32, bool) {
					if len(d) < 4 {
						return nil, false
					}
					l := float32(uint16(d[1])<<8|uint16(d[0])) / 160.0
					r := float32(uint16(d[3])<<8|uint16(d[2])) / 160.0
					return map[string]float32{
						"fuelLeftLiters":  l,
						"fuelRightLiters": r,
						"fuelLiters":      l + r,
					}, true
				},
			},
			0x330: {
				Parse: func(d []byte) (map[string]float32, bool) {
					if len(d) < 8 {
						return nil, false
					}
					odo, ok := u24le(d, 0)
					if !ok {
						return nil, false
					}
					rangeRaw, ok := u16le(d, 6)
					if !ok {
						return nil, false
					}
					rangeKm := float32(rangeRaw) / 16.0
					fuelGauge := float32(d[3])

					return map[string]float32{
						"odometerKm":      float32(odo),
						"rangeKm":         rangeKm,
						"fuelGaugeLiters": fuelGauge,
						"fuelLeftDamped":  float32(d[4]),
						"fuelRightDamped": float32(d[5]),
					}, true
				},
			},
			0x362: {
				Parse: func(d []byte) (map[string]float32, bool) {
					if len(d) < 3 {
						return nil, false
					}

					rawMpg := (int(d[2]) << 4) | int(d[1]>>4)
					avgMpgUk := float32(rawMpg) / 10.0

					rawMph := (int(d[1]&0x0F) << 8) | int(d[0])
					avgMph := float32(rawMph) / 10.0
					avgKmh := avgMph * 1.609344

					return map[string]float32{
						"avgMpgUk": avgMpgUk,
						"avgMph":   avgMph,
						"avgKmh":   avgKmh,
					}, true
				},
			},
		},
	}
}
