package store

import "sync"

const outputResourceWildcard = "*"

// ReserveOutputResources prevents independent Cue Engine instances sharing a
// Store from dispatching overlapping physical outputs at the same time. The
// reservation is process-local and is always released when execution reaches
// a terminal result; restart reconciliation remains responsible for work
// interrupted by a process restart.
func (s *Store) ReserveOutputResources(owner, entityID string, resources []string) (release func(), conflictingEntityID string) {
	if owner == "" || len(resources) == 0 {
		return func() {}, ""
	}
	if entityID == "" {
		entityID = owner
	}

	s.outputReservationLock.Lock()
	if s.outputReservations == nil {
		s.outputReservations = make(map[string]map[string]string)
	}
	for _, resource := range resources {
		for heldResource, owners := range s.outputReservations {
			if !outputResourcesOverlap(resource, heldResource) {
				continue
			}
			for heldOwner, heldEntityID := range owners {
				if heldOwner != owner {
					s.outputReservationLock.Unlock()
					return nil, heldEntityID
				}
			}
		}
	}
	for _, resource := range resources {
		owners := s.outputReservations[resource]
		if owners == nil {
			owners = make(map[string]string)
			s.outputReservations[resource] = owners
		}
		owners[owner] = entityID
	}
	s.outputReservationLock.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			s.outputReservationLock.Lock()
			defer s.outputReservationLock.Unlock()
			for resource, owners := range s.outputReservations {
				delete(owners, owner)
				if len(owners) == 0 {
					delete(s.outputReservations, resource)
				}
			}
		})
	}, ""
}

func outputResourcesOverlap(left, right string) bool {
	return left == outputResourceWildcard || right == outputResourceWildcard || left == right
}
