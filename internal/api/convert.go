package api

import (
	"gokku/internal/store"
)

func toMe(u store.User) Me {
	return Me{Id: u.ID, Username: u.Username, Role: Role(u.Role), Theme: Theme(u.Theme)}
}

func toUser(u store.User, lastActive *store.Time) User {
	out := User{
		Id:           u.ID,
		Username:     u.Username,
		Role:         Role(u.Role),
		Status:       UserStatus(u.Status),
		CreatedAt:    u.CreatedAt.Time,
		LastActiveAt: lastActive.Ptr(),
	}
	if u.Status == "invited" {
		out.InviteExpiresAt = u.InviteExpiresAt.Ptr()
	}
	return out
}

func toToken(t store.TokenListItem) Token {
	return Token{
		Id:         t.ID,
		Name:       t.Name,
		Prefix:     t.TokenPrefix,
		Role:       Role(t.Role),
		Owner:      t.Owner,
		CreatedAt:  t.CreatedAt.Time,
		LastUsedAt: t.LastUsedAt.Ptr(),
	}
}
