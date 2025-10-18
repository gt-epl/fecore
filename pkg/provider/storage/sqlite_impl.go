// Ref: https://gosamples.dev/sqlite-intro/

package storage

import (
	"database/sql"
	_ "modernc.org/sqlite"
)

type SQLiteStorageManager struct {
	db *sql.DB
}

func NewSQLiteStorageManager(db *sql.DB) (*SQLiteStorageManager, error) {
	query := `
	CREATE TABLE IF NOT EXISTS Function(
		name TEXT PRIMARY KEY UNIQUE,
		namespace TEXT,
		image TEXT,
		labels TEXT,
		annotations TEXT,
		secrets TEXT,
		secretsPath TEXT,
		envVars TEXT,
		envProcess TEXT,
		memoryLimit INT
	);
	`
	if _, err := db.Exec(query); err != nil {
		return nil, err
	}

	query = `
	CREATE TABLE IF NOT EXISTS Container(
		name TEXT PRIMARY KEY UNIQUE,
		parentFunction TEXT,
		ip TEXT
	);
	`
	if _, err := db.Exec(query); err != nil {
		return nil, err
	}
	
	return &SQLiteStorageManager{
		db: db,
	}, nil
}

func (r *SQLiteStorageManager) InsertFunction(function Function) error {
	query := `
	INSERT INTO Function(name, namespace, image, labels, annotations, secrets,
	secretsPath, envVars, envProcess, memoryLimit) 
	values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.Exec(query, function.Name, function.Namespace, function.Image, 
		function.Labels, function.Annotations, function.Secrets, function.SecretsPath, 
		function.EnvVars, function.EnvProcess, function.MemoryLimit)
	if err != nil {
		return err
	}

	return nil
}

func (r *SQLiteStorageManager) GetAllFunctions() ([]Function, error) {
	rows, err := r.db.Query("SELECT * FROM Function")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fns []Function
	for rows.Next() {
		var f Function
		if err := rows.Scan(&f.Name, &f.Namespace, &f.Image, 
		&f.Labels, &f.Annotations, &f.Secrets, &f.SecretsPath, 
		&f.EnvVars, &f.EnvProcess, &f.MemoryLimit); err != nil {
			return nil, err
		}
		fns = append(fns, f)
	}

	return fns, nil
}

func (r *SQLiteStorageManager) DeleteFunction(name string) error {
	_, err := r.db.Exec("DELETE FROM Function WHERE name = ?", name)
	if err != nil {
		return err
	}
	return nil
}

func (r *SQLiteStorageManager) InsertContainer(container Container) error {
	_, err := r.db.Exec("INSERT INTO Container(name, parentFunction, ip) values(?, ?, ?)", container.Name, container.ParentFunction, container.Ip)
	if err != nil {
		return err
	}

	return nil
}

func (r *SQLiteStorageManager) GetAllContainers() ([]Container, error) {
	rows, err := r.db.Query("SELECT * FROM Container")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var containers []Container
	for rows.Next() {
		var c Container
		if err := rows.Scan(&c.Name, &c.ParentFunction); err != nil {
			return nil, err
		}
		containers = append(containers, c)
	}

	return containers, nil
}

func (r *SQLiteStorageManager) GetContainersForFunction(name string) ([]Container, error) {
	rows, err := r.db.Query("SELECT * FROM Container WHERE parentFunction = ?", name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var containers []Container
	for rows.Next() {
		var c Container
		if err := rows.Scan(&c.Name, &c.ParentFunction, &c.Ip); err != nil {
			return nil, err
		}
		containers = append(containers, c)
	}

	return containers, nil
}

func (r *SQLiteStorageManager) DeleteContainer(name string) error {
	_, err := r.db.Exec("DELETE FROM Container WHERE name = ?", name)
	if err != nil {
		return err
	}
	return nil
}

// func Cleanup(db *sql.DB) {
// 	db.Close()
// 	os.Remove("./func.db")
// }
