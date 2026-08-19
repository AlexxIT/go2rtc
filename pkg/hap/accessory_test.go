package hap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLegacyIID pins the IIDs of the accessory shape go2rtc has always
// published. Controllers cache the accessory database, so these values must
// not change or every existing pairing is invalidated.
func TestLegacyIID(t *testing.T) {
	acc := &Accessory{
		AID: DeviceAID,
		Services: []*Service{
			ServiceAccessoryInformation("manuf", "model", "name", "serial", "firmware"),
			{
				Type: "110",
				Characters: []*Character{
					{Type: "120"},
					{Type: "114"},
					{Type: "115"},
					{Type: "116"},
					{Type: "B0"},
					{Type: "117"},
					{Type: "118"},
				},
			},
			{
				Type:       "112",
				Characters: []*Character{{Type: "11A"}},
			},
		},
	}
	acc.InitIID()

	require.Equal(t, uint64(0x1103E000), acc.Services[0].IID)
	require.Equal(t, uint64(0x1103E014), acc.Services[0].Characters[0].IID)
	require.Equal(t, uint64(0x1103E052), acc.Services[0].Characters[5].IID)

	require.Equal(t, uint64(0x11110000), acc.Services[1].IID)
	require.Equal(t, uint64(0x11110120), acc.Services[1].Characters[0].IID)
	require.Equal(t, uint64(0x11110118), acc.Services[1].Characters[6].IID)

	require.Equal(t, uint64(0x11112000), acc.Services[2].IID)
	require.Equal(t, uint64(0x1111211A), acc.Services[2].Characters[0].IID)
}

// TestSpecIID covers the services added by the HomeKit Secure Video open
// source spec. Their types are four hex digits wide, which the legacy
// positional layout cannot encode.
func TestSpecIID(t *testing.T) {
	acc := &Accessory{
		AID: DeviceAID,
		Services: []*Service{
			ServiceAccessoryInformation("manuf", "model", "name", "serial", "firmware"),
			{
				Type: "8033", // camera-webrtc-stream-management
				Characters: []*Character{
					{Type: "8053"}, // webrtc-solicit-offer
					{Type: "8054"}, // webrtc-provide-answer
					{Type: "8059"}, // webrtc-supported-video-stream-tiers
				},
			},
			{
				Type: "8031", // camera-multi-tier-rtp-stream-management
				Characters: []*Character{
					{Type: "8043"}, // supported-video-stream-tiers
					{Type: "805B"}, // sensor-uuid
				},
			},
		},
	}

	require.NotPanics(t, acc.InitIID)

	seen := map[uint64]string{}
	for _, service := range acc.Services {
		require.NotZero(t, service.IID)

		if prev, ok := seen[service.IID]; ok {
			t.Fatalf("service %s reuses IID %d of %s", service.Type, service.IID, prev)
		}
		seen[service.IID] = service.Type

		for _, character := range service.Characters {
			require.NotZero(t, character.IID)

			if prev, ok := seen[character.IID]; ok {
				t.Fatalf("characteristic %s reuses IID %d of %s", character.Type, character.IID, prev)
			}
			seen[character.IID] = character.Type
		}
	}

	// 3 services + 6 accessory information + 5 spec characteristics
	require.Len(t, seen, 14)
}

// TestGetCharacterByID guards the lookup used to dispatch reads and writes,
// which relies only on IID uniqueness.
func TestGetCharacterByID(t *testing.T) {
	acc := &Accessory{
		AID: DeviceAID,
		Services: []*Service{
			{Type: "8033", Characters: []*Character{{Type: "8053"}, {Type: "8054"}}},
		},
	}
	acc.InitIID()

	char := acc.Services[0].Characters[1]
	require.Equal(t, char, acc.GetCharacterByID(char.IID))
	require.Nil(t, acc.GetCharacterByID(0))
}

// TestMixedIID is the upgrade path: enabling the HomeKit Secure Video services
// on an accessory that is already paired must not move the IIDs of the
// services it already had, or every controller's cached database is stale.
func TestMixedIID(t *testing.T) {
	legacy := func() []*Service {
		return []*Service{
			ServiceAccessoryInformation("manuf", "model", "name", "serial", "firmware"),
			{Type: "110", Characters: []*Character{{Type: "120"}, {Type: "118"}}},
			{Type: "112", Characters: []*Character{{Type: "11A"}}},
		}
	}

	before := &Accessory{AID: DeviceAID, Services: legacy()}
	before.InitIID()

	after := &Accessory{AID: DeviceAID, Services: append(legacy(), &Service{
		Type: "8033",
		Characters: []*Character{
			{Type: "8053"},
			{Type: "8054"},
		},
	})}
	after.InitIID()

	for i, service := range before.Services {
		require.Equal(t, service.IID, after.Services[i].IID, "service %s moved", service.Type)

		for j, character := range service.Characters {
			require.Equal(
				t, character.IID, after.Services[i].Characters[j].IID,
				"characteristic %s moved", character.Type,
			)
		}
	}

	// the spec service and its characteristics are numbered below every
	// possible positional IID, so they cannot collide with the legacy ones
	spec := after.Services[len(after.Services)-1]
	require.Equal(t, uint64(1), spec.IID)
	require.Equal(t, uint64(2), spec.Characters[0].IID)
	require.Equal(t, uint64(3), spec.Characters[1].IID)

	seen := map[uint64]bool{}
	for _, service := range after.Services {
		require.Less(t, spec.Characters[1].IID, uint64(minLegacyIID))
		require.False(t, seen[service.IID])
		seen[service.IID] = true

		for _, character := range service.Characters {
			require.False(t, seen[character.IID], "IID %d reused", character.IID)
			seen[character.IID] = true
		}
	}
}

// TestSpecCharacterOnLegacyService covers a four hex digit characteristic added
// to a service that keeps its positional IID.
func TestSpecCharacterOnLegacyService(t *testing.T) {
	acc := &Accessory{
		AID: DeviceAID,
		Services: []*Service{
			{Type: "110", Characters: []*Character{{Type: "120"}, {Type: "8041"}}},
		},
	}
	acc.InitIID()

	require.Equal(t, uint64(0x11110000), acc.Services[0].IID)
	require.Equal(t, uint64(0x11110120), acc.Services[0].Characters[0].IID)
	require.Equal(t, uint64(1), acc.Services[0].Characters[1].IID)
}
