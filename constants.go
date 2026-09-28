package replikator

import "time"

const (
	// FieldManagerReplikator is the server-side apply field manager recorded on replicas.
	FieldManagerReplikator = "io.github.yankeguo/replikator"

	// AnnotationManaged marks an object as a replica owned by this process.
	AnnotationManaged = "replikator.yankeguo.github.io/managed"
	// AnnotationSourceNamespace is the namespace of the object this replica was copied from.
	AnnotationSourceNamespace = "replikator.yankeguo.github.io/source-namespace"
	// AnnotationSourceName is the name of the object this replica was copied from.
	AnnotationSourceName = "replikator.yankeguo.github.io/source-name"

	defaultFullSyncInterval   = 10 * time.Minute
	defaultWatchRetryInterval = 5 * time.Second
	watchRestartInterval      = time.Second
	defaultPollInterval       = 10 * time.Second
	javascriptTimeout         = 2 * time.Second
)
