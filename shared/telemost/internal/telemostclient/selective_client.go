// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package telemostclient

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"math/rand/v2"
	"sort"
	"strings"
	"time"
	"whitelist-bypass/relay/bond"
)

//go:embed invitations-selective.txt
var selectiveInvitations string

func selectiveGroups(text string) ([][]string, error) {
	links := strings.Fields(text)
	if len(links) == 0 || len(links)%bond.SelectiveLanes != 0 {
		return nil, fmt.Errorf("пул должен состоять из полных пар")
	}
	seen := map[string]bool{}
	groups := [][]string{}
	for i := 0; i < len(links); i += bond.SelectiveLanes {
		g := links[i : i+bond.SelectiveLanes]
		if _, e := bond.GroupKey(g); e != nil {
			return nil, e
		}
		for _, link := range g {
			if seen[link] {
				return nil, fmt.Errorf("приглашение повторяется в разных парах")
			}
			seen[link] = true
		}
		groups = append(groups, g)
	}
	return groups, nil
}
func RunAutomaticContext(ctx context.Context, addr string) error {
	return RunSelectiveContext(ctx, addr, bond.DefaultSelectiveProfile())
}

// RunSelectiveContext используется Linux-испытателем. Android выбирает профиль
// внутри программы и запускается без аргументов и без прямого доступа к VPS.
func RunSelectiveContext(ctx context.Context, addr string, profile bond.SelectiveProfile) error {
	return RunSelectiveDiagnostics(ctx, addr, profile, nil)
}
func RunSelectiveDiagnostics(ctx context.Context, addr string, profile bond.SelectiveProfile, hook func(*bond.Transport)) error {
	return RunSelectiveLinksDiagnostics(ctx, addr, profile, strings.Fields(selectiveInvitations), hook)
}

// RunSelectiveLinksDiagnostics использует упорядоченные пары из подписки.
func RunSelectiveLinksDiagnostics(ctx context.Context, addr string, profile bond.SelectiveProfile, links []string, hook func(*bond.Transport)) error {
	if !profile.Valid() {
		return fmt.Errorf("некорректный профиль основного и резервного путей")
	}
	groups, e := selectiveGroups(strings.Join(links, "\n"))
	if e != nil {
		return e
	}
	preferred := preferredPair(links, len(groups))
	cooldown := map[int]time.Time{}
	for ctx.Err() == nil {
		order := rand.Perm(len(groups))
		sort.SliceStable(order, func(a, b int) bool {
			x, y := order[a], order[b]
			xc, yc := time.Now().Before(cooldown[x]), time.Now().Before(cooldown[y])
			if xc != yc {
				return !xc
			}
			if xc {
				return cooldown[x].Before(cooldown[y])
			}
			return x == preferred && y != preferred
		})
		picked, active, err := selectPair(ctx, order, 600*time.Millisecond, func(candidate context.Context, index int, commit func() bool) (bool, error) {
			log.Printf("Проверяю свободную пару %d/%d через Телемост", index+1, len(groups))
			return runReliableAttemptSelected(candidate, groups[index], addr, index+1, &profile, commit, func(t *bond.Transport) {
				rememberPair(links, index)
				if hook != nil {
					hook(t)
				}
			})
		}, func(index int) {
			log.Printf("Выбрана пара %d; остальные проверки отменены, подключаю резерв", index+1)
		})
		if err != nil {
			return err
		}
		if picked >= 0 {
			if active {
				preferred = picked
				delete(cooldown, picked)
			} else {
				cooldown[picked] = time.Now().Add(30 * time.Second)
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(2 * time.Second):
		}
	}
	return nil
}
