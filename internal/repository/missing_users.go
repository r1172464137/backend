package repository

import "context"

// MissingUserIDs returns the IDs among ids for which no account exists.
func MissingUserIDs(ctx context.Context, users UserRepo, ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	found, err := users.FindUsersByIds(ctx, ids)
	if err != nil {
		return nil, err
	}
	exists := make(map[int64]bool, len(found))
	for _, u := range found {
		if u != nil {
			exists[u.Id] = true
		}
	}
	var missing []int64
	for _, id := range ids {
		if !exists[id] {
			missing = append(missing, id)
		}
	}
	return missing, nil
}
