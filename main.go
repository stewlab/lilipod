/* SPDX-License-Identifier: GPL-3.0-only

This file is part of the lilipod project:
   https://github.com/89luca89/lilipod

Copyright (C) 2023 lilipod contributors

lilipod is free software; you can redistribute it and/or modify it
under the terms of the GNU General Public License version 3
as published by the Free Software Foundation.

lilipod is distributed in the hope that it will be useful, but
WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU
General Public License for more details.

You should have received a copy of the GNU General Public License
along with lilipod; if not, see <http://www.gnu.org/licenses/>. */

package main

import (
	"log"

	"github.com/89luca89/lilipod/cmd"
)

func main() {
	if err := cmd.InitEnvironment(); err != nil {
		log.Fatalf("%+v\n", err)
	}

	app := cmd.NewApp()
	if err := app.Execute(); err != nil {
		log.Fatalf("%+v\n", err)
	}
}
