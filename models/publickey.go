// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Unknwon/com"
)

var (
	sshOpLocker = sync.Mutex{}
	//publicKeyRootPath string
	sshPath string
	appPath string
	// "### autogenerated by gitgos, DO NOT EDIT\n"
	tmplPublicKey = "command=\"%s serv key-%d\",no-port-forwarding," +
		"no-X11-forwarding,no-agent-forwarding,no-pty %s\n"
)

func exePath() (string, error) {
	file, err := exec.LookPath(os.Args[0])
	if err != nil {
		return "", err
	}
	return filepath.Abs(file)
}

func homeDir() string {
	home, err := com.HomeDir()
	if err != nil {
		return "/"
	}
	return home
}

func init() {
	var err error
	appPath, err = exePath()
	if err != nil {
		println(err.Error())
		os.Exit(2)
	}

	sshPath = filepath.Join(homeDir(), ".ssh")
}

type PublicKey struct {
	Id      int64
	OwnerId int64     `xorm:"index"`
	Name    string    `xorm:"unique not null"`
	Content string    `xorm:"text not null"`
	Created time.Time `xorm:"created"`
	Updated time.Time `xorm:"updated"`
}

func GenAuthorizedKey(keyId int64, key string) string {
	return fmt.Sprintf(tmplPublicKey, appPath, keyId, key)
}

func AddPublicKey(key *PublicKey) error {
	_, err := orm.Insert(key)
	if err != nil {
		return err
	}

	err = SaveAuthorizedKeyFile(key)
	if err != nil {
		_, err2 := orm.Delete(key)
		if err2 != nil {
			// TODO: log the error
		}
		return err
	}

	return nil
}

// DeletePublicKey deletes SSH key information both in database and authorized_keys file.
func DeletePublicKey(key *PublicKey) (err error) {
	has, err := orm.Id(key.Id).Get(key)
	if err != nil {
		return err
	} else if !has {
		return errors.New("Public key does not exist")
	}
	if _, err = orm.Delete(key); err != nil {
		return err
	}

	sshOpLocker.Lock()
	defer sshOpLocker.Unlock()

	p := filepath.Join(sshPath, "authorized_keys")
	tmpP := filepath.Join(sshPath, "authorized_keys.tmp")
	fr, err := os.Open(p)
	if err != nil {
		return err
	}
	defer fr.Close()

	fw, err := os.Create(tmpP)
	if err != nil {
		return err
	}
	defer fw.Close()

	buf := bufio.NewReader(fr)
	for {
		line, errRead := buf.ReadString('\n')
		line = strings.TrimSpace(line)

		if errRead != nil {
			if errRead != io.EOF {
				return errRead
			}

			// Reached end of file, if nothing to read then break,
			// otherwise handle the last line.
			if len(line) == 0 {
				break
			}
		}

		// Found the line and copy rest of file.
		if strings.Contains(line, key.Content) {
			continue
		}
		// Still finding the line, copy the line that currently read.
		if _, err = fw.WriteString(line + "\n"); err != nil {
			return err
		}

		if errRead == io.EOF {
			break
		}
	}

	if err = os.Remove(p); err != nil {
		return err
	}

	return os.Rename(tmpP, p)
}

func ListPublicKey(userId int64) ([]PublicKey, error) {
	keys := make([]PublicKey, 0)
	err := orm.Find(&keys, &PublicKey{OwnerId: userId})
	return keys, err
}

func SaveAuthorizedKeyFile(key *PublicKey) error {
	sshOpLocker.Lock()
	defer sshOpLocker.Unlock()

	p := filepath.Join(sshPath, "authorized_keys")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	//os.Chmod(p, 0600)
	_, err = f.WriteString(GenAuthorizedKey(key.Id, key.Content))
	return err
}
