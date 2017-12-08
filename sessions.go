package boltdb

import (
	"errors"
	"sync"

	"encoding/binary"

	"github.com/VolantMQ/persistence"
	"github.com/boltdb/bolt"
)

type sessions struct {
	db *dbStatus

	// transactions that are in progress right now
	wgTx *sync.WaitGroup
	lock *sync.Mutex
}

var _ persistence.Sessions = (*sessions)(nil)

func (s *sessions) init() error {
	return s.db.db.Update(func(tx *bolt.Tx) error {
		sessions := tx.Bucket(bucketSessions)
		if sessions == nil {
			return persistence.ErrNotInitialized
		}

		val := sessions.Get(sessionsCount)
		if len(val) == 0 {
			buf := [8]byte{}
			num := binary.PutUvarint(buf[:], 0)
			sessions.Put(sessionsCount, buf[:num]) // nolint: errcheck
		}

		return nil
	})
}

func (s *sessions) Exists(id []byte) bool {
	err := s.db.db.View(func(tx *bolt.Tx) error {
		sessions := tx.Bucket(bucketSessions)
		if sessions == nil {
			return persistence.ErrNotInitialized
		}

		ses := sessions.Bucket(id)
		if ses == nil {
			return bolt.ErrBucketNotFound
		}

		return nil
	})

	return err == nil
}

func (s *sessions) Count() uint64 {
	var count uint64
	s.db.db.View(func(tx *bolt.Tx) error { // nolint: errcheck
		sessions := tx.Bucket(bucketSessions)

		val := sessions.Get(sessionsCount)
		if cnt, num := binary.Uvarint(val); num > 0 {
			count = cnt
		}

		return nil
	})

	return count
}

func (s *sessions) SubscriptionsStore(id []byte, data []byte) error {
	return s.db.db.Update(func(tx *bolt.Tx) error {
		sessions := tx.Bucket(bucketSessions)
		if sessions == nil {
			return persistence.ErrNotInitialized
		}

		session, err := getSession(id, sessions)
		if err != nil {
			return err
		}

		return session.Put(bucketSubscriptions, data)
	})
}

func (s *sessions) SubscriptionsDelete(id []byte) error {
	return s.db.db.Update(func(tx *bolt.Tx) error {
		sessions := tx.Bucket(bucketSessions)
		if sessions == nil {
			return persistence.ErrNotInitialized
		}

		session := tx.Bucket(id)
		if session == nil {
			return persistence.ErrNotFound
		}

		session.Delete(bucketSubscriptions) // nolint: errcheck
		return nil
	})
}

func boolToByte(v bool) byte {
	if v {
		return 1
	}
	return 0
}

func byteToBool(v byte) bool {
	return !(v == 0)
}

func (s *sessions) PacketsForEach(id []byte, loader persistence.PacketLoader) error {
	return s.db.db.View(func(tx *bolt.Tx) error {
		root := tx.Bucket(bucketSessions)
		if root == nil {
			return persistence.ErrNotInitialized
		}

		session := root.Bucket(id)
		if session == nil {
			return nil
		}

		packs := session.Bucket(bucketPackets)
		if packs == nil {
			return nil
		}

		packs.ForEach(func(k, v []byte) error { // nolint: errcheck
			if packet := packs.Bucket(k); packet != nil {
				pPkt := persistence.PersistedPacket{UnAck: false}

				if data := packet.Get([]byte("data")); len(data) > 0 {
					pPkt.Data = data
				} else {
					return persistence.ErrBrokenEntry
				}

				if data := packet.Get([]byte("unAck")); len(data) > 0 {
					pPkt.UnAck = byteToBool(data[0])
				}

				if data := packet.Get([]byte("expireAt")); len(data) > 0 {
					pPkt.ExpireAt = string(data)
				}

				return loader.LoadPersistedPacket(pPkt)
			}
			return nil
		})

		return nil
	})
}

func (s *sessions) PacketsStore(id []byte, packets []persistence.PersistedPacket) error {
	return s.db.db.Update(func(tx *bolt.Tx) error {
		buck, err := createPacketsBucket(tx, id)
		if err != nil {
			return err
		}

		for _, entry := range packets {
			if err = storePacket(buck, entry); err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *sessions) PacketStore(id []byte, packet persistence.PersistedPacket) error {
	err := s.db.db.Update(func(tx *bolt.Tx) error {
		buck, err := createPacketsBucket(tx, id)
		if err != nil {
			return err
		}

		return storePacket(buck, packet)
	})

	return err
}

func (s *sessions) PacketsDelete(id []byte) error {
	return s.db.db.Update(func(tx *bolt.Tx) error {
		if sessions := tx.Bucket(bucketSessions); sessions != nil {
			ses := sessions.Bucket(id)
			if ses == nil {
				return persistence.ErrNotFound
			}
			ses.DeleteBucket(bucketPackets) // nolint: errcheck
		}
		return nil
	})
}

func (s *sessions) LoadForEach(loader persistence.SessionLoader, context interface{}) error {
	return s.db.db.Update(func(tx *bolt.Tx) error {
		sessions := tx.Bucket(bucketSessions)
		if sessions == nil {
			return nil
		}

		return sessions.ForEach(func(k, v []byte) error {
			// If there's a value, it's not a bucket so ignore it.
			if v != nil {
				return nil
			}

			session := sessions.Bucket(k)
			st := &persistence.SessionState{}

			state := session.Bucket(bucketState)
			if state != nil {
				if v := state.Get([]byte("version")); len(v) > 0 {
					st.Version = v[0]
				} else {
					st.Errors = append(st.Errors, errors.New("protocol version not found"))
				}

				st.Timestamp = string(state.Get([]byte("timestamp")))
				st.Subscriptions = state.Get(bucketSubscriptions)

				if expire := state.Bucket(bucketExpire); expire != nil {
					st.Expire = &persistence.SessionDelays{
						Since:    string(expire.Get([]byte("since"))),
						ExpireIn: string(expire.Get([]byte("expireIn"))),
						WillIn:   string(expire.Get([]byte("willIn"))),
						WillData: expire.Get([]byte("willData")),
					}
				}
			}
			return loader.LoadSession(context, k, st)
		})
	})
}

func (s *sessions) StateStore(id []byte, state *persistence.SessionState) error {
	return s.db.db.Update(func(tx *bolt.Tx) error {
		sessions := tx.Bucket(bucketSessions)
		if sessions == nil {
			return persistence.ErrNotInitialized
		}

		session, err := getSession(id, sessions)
		if err != nil {
			return err
		}

		var st *bolt.Bucket
		st, err = session.CreateBucketIfNotExists(bucketState)
		if err != nil {
			return err
		}

		if len(state.Subscriptions) > 0 {
			if err = st.Put(bucketSubscriptions, state.Subscriptions); err != nil {
				return err
			}
		}

		if err = st.Put([]byte("timestamp"), []byte(state.Timestamp)); err != nil {
			return err
		}

		if state.Expire != nil {
			expire, err := st.CreateBucketIfNotExists(bucketExpire)
			if err != nil {
				return err
			}

			if err = expire.Put([]byte("since"), []byte(state.Expire.Since)); err != nil {
				return err
			}

			if len(state.Expire.ExpireIn) > 0 {
				if err = expire.Put([]byte("expireIn"), []byte(state.Expire.ExpireIn)); err != nil {
					return err
				}
			}

			if len(state.Expire.WillIn) > 0 {
				if err = expire.Put([]byte("willIn"), []byte(state.Expire.WillIn)); err != nil {
					return err
				}
			}
			if len(state.Expire.WillData) > 0 {
				if err = expire.Put([]byte("willData"), state.Expire.WillData); err != nil {
					return err
				}
			}
		}

		return nil
	})
}

func (s *sessions) StateDelete(id []byte) error {
	return s.db.db.Update(func(tx *bolt.Tx) error {
		sessions := tx.Bucket(bucketSessions)
		if sessions == nil {
			return persistence.ErrNotInitialized
		}

		session, err := getSession(id, sessions)
		if err != nil {
			return err
		}

		session.DeleteBucket(bucketState) // nolint: errcheck

		return nil
	})
}

func (s *sessions) Delete(id []byte) error {
	return s.db.db.Update(func(tx *bolt.Tx) error {
		sessions := tx.Bucket(bucketSessions)
		if sessions == nil {
			return persistence.ErrNotInitialized
		}

		session := sessions.Bucket(id)
		if session == nil {
			return persistence.ErrNotFound
		}

		if err := sessions.DeleteBucket(id); err != nil {
			return err
		}

		count, num := binary.Uvarint(sessions.Get(sessionsCount))
		if num <= 0 {
			return errors.New("persistence: broken count")
		}

		if count == 0 {
			return errors.New("persistence: count already 0")
		}

		count--
		buf := [8]byte{}
		num = binary.PutUvarint(buf[:], count)
		sessions.Put(sessionsCount, buf[:num]) // nolint: errcheck

		return nil
	})
}

func getSession(id []byte, sessions *bolt.Bucket) (*bolt.Bucket, error) {
	session := sessions.Bucket(id)
	if session == nil {
		var err error
		if session, err = sessions.CreateBucket(id); err != nil {
			return nil, err
		}

		count, num := binary.Uvarint(sessions.Get(sessionsCount))
		if num <= 0 {
			sessions.DeleteBucket(id) // nolint: errcheck
			return nil, nil
		}

		count++

		buf := [8]byte{}
		num = binary.PutUvarint(buf[:], count)
		sessions.Put([]byte("count"), buf[:num]) // nolint: errcheck
	}

	return session, nil
}

func createPacketsBucket(tx *bolt.Tx, id []byte) (*bolt.Bucket, error) {
	sessions, err := tx.CreateBucketIfNotExists(bucketSessions)
	if err != nil {
		return nil, err
	}

	var session *bolt.Bucket
	if session, err = getSession(id, sessions); err != nil {
		return nil, err
	}

	return session.CreateBucketIfNotExists(bucketPackets)
}

func storePacket(buck *bolt.Bucket, packet persistence.PersistedPacket) error {
	id, _ := buck.NextSequence() // nolint: gas
	pBuck, err := buck.CreateBucketIfNotExists(itob64(id))
	if err != nil {
		return err
	}

	if err = pBuck.Put([]byte("data"), packet.Data); err != nil {
		return err
	}

	if err = pBuck.Put([]byte("unAck"), []byte{boolToByte(packet.UnAck)}); err != nil {
		return err
	}

	if len(packet.ExpireAt) > 0 {
		if err = pBuck.Put([]byte("expireAt"), []byte(packet.ExpireAt)); err != nil {
			return err
		}
	}

	return nil
}
