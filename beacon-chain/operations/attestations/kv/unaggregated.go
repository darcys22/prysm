package kv

import (
	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	ethpb "github.com/prysmaticlabs/ethereumapis/eth/v1alpha1"
	"github.com/prysmaticlabs/go-bitfield"
	"github.com/prysmaticlabs/prysm/beacon-chain/core/helpers"
	stateTrie "github.com/prysmaticlabs/prysm/beacon-chain/state"
)

// SaveUnaggregatedAttestation saves an unaggregated attestation in cache.
func (p *AttCaches) SaveUnaggregatedAttestation(att *ethpb.Attestation) error {
	if att == nil {
		return nil
	}
	if helpers.IsAggregated(att) {
		return errors.New("attestation is aggregated")
	}

	r, err := hashFn(att.Data)
	if err != nil {
		return errors.Wrap(err, "could not tree hash attestation")
	}

	// Don't save the attestation if the bitfield has been contained in previous blocks.
	v, ok := p.seenAggregatedAtt.Get(string(r[:]))
	if ok {
		seenBits, ok := v.([]bitfield.Bitlist)
		if !ok {
			return errors.New("could not convert to bitlist type")
		}
		for _, bit := range seenBits {
			if bit.Len() == att.AggregationBits.Len() && bit.Contains(att.AggregationBits) {
				return nil
			}
		}
	}

	r, err = hashFn(att)
	if err != nil {
		return errors.Wrap(err, "could not tree hash attestation")
	}
	p.unAggregateAttLock.Lock()
	defer p.unAggregateAttLock.Unlock()
	p.unAggregatedAtt[r] = stateTrie.CopyAttestation(att) // Copied.

	return nil
}

// SaveUnaggregatedAttestations saves a list of unaggregated attestations in cache.
func (p *AttCaches) SaveUnaggregatedAttestations(atts []*ethpb.Attestation) error {
	for _, att := range atts {
		if err := p.SaveUnaggregatedAttestation(att); err != nil {
			return err
		}
	}

	return nil
}

// UnaggregatedAttestations returns all the unaggregated attestations in cache.
func (p *AttCaches) UnaggregatedAttestations() ([]*ethpb.Attestation, error) {
	p.unAggregateAttLock.Lock()
	defer p.unAggregateAttLock.Unlock()
	unAggregatedAtts := p.unAggregatedAtt
	atts := make([]*ethpb.Attestation, 0, len(unAggregatedAtts))
	for _, att := range unAggregatedAtts {
		r, err := hashFn(att.Data)
		if err != nil {
			return nil, errors.Wrap(err, "could not tree hash attestation")
		}
		v, ok := p.seenAggregatedAtt.Get(string(r[:]))
		if ok {
			seenBits, ok := v.([]bitfield.Bitlist)
			if !ok {
				return nil, errors.New("could not convert to bitlist type")
			}
			for _, bit := range seenBits {
				if bit.Len() == att.AggregationBits.Len() && bit.Contains(att.AggregationBits) {
					r, err := hashFn(att)
					if err != nil {
						return nil, errors.Wrap(err, "could not tree hash attestation")
					}
					delete(p.unAggregatedAtt, r)
					continue
				}
			}
		}

		atts = append(atts, stateTrie.CopyAttestation(att) /* Copied */)
	}

	return atts, nil
}

// UnaggregatedAttestationsBySlotIndex returns the unaggregated attestations in cache,
// filtered by committee index and slot.
func (p *AttCaches) UnaggregatedAttestationsBySlotIndex(slot uint64, committeeIndex uint64) []*ethpb.Attestation {
	atts := make([]*ethpb.Attestation, 0)

	p.unAggregateAttLock.RLock()
	defer p.unAggregateAttLock.RUnlock()

	unAggregatedAtts := p.unAggregatedAtt
	for _, a := range unAggregatedAtts {
		if slot == a.Data.Slot && committeeIndex == a.Data.CommitteeIndex {
			atts = append(atts, a)
		}
	}

	return atts
}

// DeleteUnaggregatedAttestation deletes the unaggregated attestations in cache.
func (p *AttCaches) DeleteUnaggregatedAttestation(att *ethpb.Attestation) error {
	if att == nil {
		return nil
	}
	if helpers.IsAggregated(att) {
		return errors.New("attestation is aggregated")
	}

	r, err := hashFn(att)
	if err != nil {
		return errors.Wrap(err, "could not tree hash attestation")
	}

	p.unAggregateAttLock.Lock()
	defer p.unAggregateAttLock.Unlock()
	delete(p.unAggregatedAtt, r)

	r, err = hashFn(att.Data)
	if err != nil {
		return errors.Wrap(err, "could not tree hash attestation data")
	}
	v, ok := p.seenAggregatedAtt.Get(string(r[:]))
	if ok {
		seenBits, ok := v.([]bitfield.Bitlist)
		if !ok {
			return errors.New("could not convert to bitlist type")
		}
		seenBits = append(seenBits, att.AggregationBits)
		p.seenAggregatedAtt.Set(string(r[:]), seenBits, cache.DefaultExpiration)
	} else {
		p.seenAggregatedAtt.Set(string(r[:]), []bitfield.Bitlist{att.AggregationBits}, cache.DefaultExpiration)
	}

	return nil
}

// UnaggregatedAttestationCount returns the number of unaggregated attestations key in the pool.
func (p *AttCaches) UnaggregatedAttestationCount() int {
	p.unAggregateAttLock.RLock()
	defer p.unAggregateAttLock.RUnlock()
	return len(p.unAggregatedAtt)
}
