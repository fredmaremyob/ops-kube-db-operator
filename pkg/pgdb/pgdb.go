package pgdb

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	dfm "github.com/MYOB-Technology/dataform/pkg/db"
	"github.com/MYOB-Technology/ops-kube-db-operator/pkg/apis/postgresdb/v1alpha1"
	dbClientSet "github.com/MYOB-Technology/ops-kube-db-operator/pkg/client/clientset/versioned"
	"github.com/MYOB-Technology/ops-kube-db-operator/pkg/secret"
	"k8s.io/client-go/kubernetes"
)

const (
	pgPollIntervalSeconds time.Duration = 60
	pgPollTimeoutSeconds  time.Duration = 1800

	ADMIN_SECRET_POSTFIX  = "-admin"
	MASTER_SECRET_POSTFIX = "-master"

	// TODO: When we start using env vars, make this configurable!!!!
	OPERATOR_NAMESPACE = "kube-system"
)

// PgDB represents a Kubernetes PostgresDB resource
type PgDB struct {
	obj      *v1alpha1.PostgresDB
	klient   *kubernetes.Clientset
	dbklient dbClientSet.Interface
	exists   bool
	ns       string
	db       *dfm.DB
	rds      *dfm.Manager
}

// Save updates a postgresdb when it exists, creates a new one if it doesnt
func (p *PgDB) Save() error {
	var err error

	if p.obj.Status.ARN == "" {
		err = p.configureNewDB()
		if err != nil {
			return err
		}
		log.Infof("creating DB with ID: %s", *p.db.Name)
		return p.Create()
	}
	return p.Stat()
}

func (p *PgDB) Create() error {
	var err error
	p.db, err = p.rds.Create(p.db)
	if err != nil {
		return err
	}
	go p.PollStatus()

	return p.Stat()
}

// PollStatus polls for status of rds instance
func (p *PgDB) PollStatus() {
	status := p.rds.WaitForFinalState(*p.db.Name, pgPollIntervalSeconds, pgPollTimeoutSeconds)
	for poll := range status {
		if poll.Err != nil {
			log.Warnf("rds instance %s transitioned to error condition: %v", *p.db.Name, poll.Err)
			p.Stat()
			return
		}
		log.Infof("poll instance %s: %s", *p.db.Name, poll.Status)
		p.Stat()
	}
}

// StatDB checks for existence of RDS instance  only
func (p *PgDB) StatDB() error {
	// list the db and get status
	_, err := p.rds.Stat(*p.db.Name)
	if err != nil {
		log.Infof("stat instance not found %s: %v", *p.db.Name, err)
		return err
	}
	return nil
}

// Stat checks for existence of RDS instance and updates db info and kubes resource Status
func (p *PgDB) Stat() error {
	// list the db and get status
	instance, err := p.rds.Stat(*p.db.Name)
	if err != nil {
		log.Infof("stat instance not found %s: %v", *p.db.Name, err)
		return err
	}
	log.Infof("instance found %s: %s", *p.db.Name, *instance.ARN)

	p.obj.Status.Ready = *instance.Status
	p.obj.Status.ARN = *instance.ARN
	p.db = instance
	p.updateDBSecret()
	log.Infof("stat updating postgresdb resource %s/%s", p.ns, p.obj.ObjectMeta.Name)
	var obj *v1alpha1.PostgresDB
	obj, err = p.dbklient.Postgresdb().PostgresDBs(p.ns).Update(p.obj)
	if err != nil {
		log.Errorf("error stat updating postgresdb resource %s/%s: %v", p.ns, p.obj.ObjectMeta.Name, err)
		return err
	}
	p.obj = obj
	log.Infof("saved postgresdb %s/%s, status: %s", p.ns, p.obj.ObjectMeta.Name, p.obj.Status.Ready)
	return nil
}

// Delete deletes a postgresdb resource from Kubernetes
func (p *PgDB) Delete() error {
	// p.obj is expected to not exist - just check for rds existence
	if err := p.StatDB(); err == nil {
		_, err := p.rds.Delete(*p.db.Name)
		if err != nil {
			return err
		}
		log.Warnf("deleted postgresdb rds instance: %s", *p.db.Name)
	}
	return nil
}

// updateEndpoint will store the endpoint as a secret when db address and port are available
// TODO: this should return an error for error handling
func (p *PgDB) updateDBSecret() {
	if p.db.Address != nil && p.db.Port != nil {
		secretName := p.obj.Name + ADMIN_SECRET_POSTFIX

		_, sec, err := secret.NewOrGet(p.klient, p.obj.Namespace, secretName)

		if err != nil {
			log.Errorf("error getting or creating secret: %s: %s", secretName, err)
			return
		}

		sec.Host = *p.db.Address
		sec.Port = "5432"

		err = secret.SaveOrCreate(p.klient, sec)
		if err != nil {
			log.Errorf("error storing DB secret: %s, %s", sec, err)
			return
		}

		log.Infof("successfully stored DB secret: %s", sec)

		sec.Name = p.obj.Name + MASTER_SECRET_POSTFIX
		sec.Namespace = OPERATOR_NAMESPACE
		err = secret.SaveOrCreate(p.klient, sec)
		if err != nil {
			log.Errorf("error storing DB secret: %s, %s", sec, err)
			return
		}
		log.Infof("successfully stored DB secret: %s", sec)
	}
}

func (p *PgDB) configureNewDB() error {
	log.Infof("configuring new db: %s", p.obj.Name)

	username := p.rds.GenerateRandomUsername(16)
	password := p.rds.GenerateRandomPassword(32)

	if p.db.Name == nil {
		return fmt.Errorf("error DB name is not set for DB: %s", p.obj.Name)
	}

	// create secret with some info
	defer func() error {
		_, newSec, err := secret.NewOrGet(p.klient, p.obj.Namespace, p.obj.Name+ADMIN_SECRET_POSTFIX)

		if err != nil {
			return fmt.Errorf("error storing DB master credentials for DB: %s, %s", p.obj.Name, err)
		}

		newSec.DatabaseName = "postgres"
		newSec.Username = username
		newSec.Password = password

		err = secret.SaveOrCreate(p.klient, newSec)

		newSec.Name = p.obj.Name + MASTER_SECRET_POSTFIX
		newSec.Namespace = OPERATOR_NAMESPACE
		err = secret.SaveOrCreate(p.klient, newSec)

		if err != nil {
			return fmt.Errorf("error storing DB master credentials: %s: %s", newSec, err)
		}

		log.Infof("successfully stored DB master credentials: %s", newSec)
		return nil
	}()

	tags := make([]*dfm.Tag, 0, 5)
	// add namespace tag
	tags = append(tags, p.tag("Namespace", p.obj.Namespace))
	// add name tag
	tags = append(tags, p.tag("Resource", p.obj.Name))
	// add created by tag
	tags = append(tags, p.tag("Created By", "DB controller"))
	// TODO: add controller version tag
	// TODO: add cluster name or identifier tag

	p.db.MasterUsername = &username
	p.db.MasterUserPassword = &password
	p.db.Tags = tags
	if p.obj.Spec.Size != "" {
		p.db.DBInstanceClass = &p.obj.Spec.Size
	}

	// retrieve gigabytes if provided
	var dbStorageAllocatedGB int64
	if p.obj.Spec.Storage > 0 {
		dbStorageAllocatedGB = p.obj.Spec.Storage
		p.db.StorageAllocatedGB = &dbStorageAllocatedGB
	}

	// retrieve iops if provided
	var dbStorageIops int64
	var dbStorageType string
	if p.obj.Spec.Iops > 0 {
		dbStorageIops = p.obj.Spec.Iops
		p.db.StorageIops = &dbStorageIops
		// if iops is provided, explicitly set the storage type to io1
		dbStorageType = "io1"
		p.db.StorageType = &dbStorageType
	}

	return nil

}

func (p *PgDB) tag(key, val string) *dfm.Tag {
	return &dfm.Tag{
		Key:   &key,
		Value: &val,
	}
}
