package moe.matsuri.nb4a.utils
fun String.listByLineOrComma(): List<String> = split(",", "\n").map { it.trim() }.filter { it.isNotEmpty() }
