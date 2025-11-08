# Atlas configuration for recap-db migrations

env "local" {
  src = "file://schema.hcl"
  url = "postgres://recap_user:recap_db_pass_DO_NOT_USE_THIS@localhost:5435/recap?sslmode=disable"
  dev = "postgres://postgres:password@localhost:5433/atlas_dev?sslmode=disable"

  migration {
    dir = "file://migrations"
  }

  format {
    migrate {
      diff = "{{ sql . \"  \" }}"
    }
  }
}

env "kubernetes" {
  src = "file://schema.hcl"
  url = env("DATABASE_URL")

  migration {
    dir      = "file://migrations"
    baseline = "20240101000300"
  }

  diff {
    concurrent_index {
      create = false
      drop   = false
    }
  }

  format {
    migrate {
      diff = "{{ sql . \"  \" }}"
    }
  }

  lint {
    destructive {
      error = true
    }
  }
}

env "default" {
  for_each = toset(["local", "kubernetes"])
  url      = atlas.env[each.key].url
  src      = atlas.env[each.key].src

  migration {
    dir = atlas.env[each.key].migration.dir
  }
}

exec {
  schema = ["public"]
}

format {
  migrate {
    apply = format(
      "-- Migration: %s\n-- Created: %s\n-- Atlas Version: %s\n\n%s",
      "{{ .Name }}",
      "{{ .Time }}",
      "{{ .Version }}",
      "{{ sql . \"  \" }}"
    )
  }
}
