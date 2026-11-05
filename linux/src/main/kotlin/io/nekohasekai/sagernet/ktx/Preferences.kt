package io.nekohasekai.sagernet.ktx

import io.nekohasekai.sagernet.database.PreferenceStore
import kotlin.reflect.KProperty

fun PreferenceStore.string(
    name: String,
    defaultValue: () -> String = { "" },
) = PreferenceProxy(name, defaultValue, ::getString, ::putString)

fun PreferenceStore.boolean(
    name: String,
    defaultValue: () -> Boolean = { false },
) = PreferenceProxy(name, defaultValue, ::getBoolean, ::putBoolean)

fun PreferenceStore.int(
    name: String,
    defaultValue: () -> Int = { 0 },
) = PreferenceProxy(name, defaultValue, ::getInt, ::putInt)

fun PreferenceStore.stringSet(
    name: String,
    defaultValue: () -> Set<String> = { setOf() },
) = PreferenceProxy(name, defaultValue, ::getStringSet, ::putStringSet)

fun PreferenceStore.stringToInt(
    name: String,
    defaultValue: () -> Int = { 0 },
) = PreferenceProxy(name, defaultValue, { key, default ->
    getString(key, "$default")?.toIntOrNull() ?: default
}, { key, value -> putString(key, "$value") })

fun PreferenceStore.stringToIntIfExists(
    name: String,
    defaultValue: () -> Int = { 0 },
) = PreferenceProxy(name, defaultValue, { key, default ->
    getString(key, "$default")?.toIntOrNull() ?: default
}, { key, value -> putString(key, value.takeIf { it > 0 }?.toString() ?: "") })

fun PreferenceStore.long(
    name: String,
    defaultValue: () -> Long = { 0L },
) = PreferenceProxy(name, defaultValue, ::getLong, ::putLong)

fun PreferenceStore.stringToLong(
    name: String,
    defaultValue: () -> Long = { 0L },
) = PreferenceProxy(name, defaultValue, { key, default ->
    getString(key, "$default")?.toLongOrNull() ?: default
}, { key, value -> putString(key, "$value") })

class PreferenceProxy<T>(
    val name: String,
    val defaultValue: () -> T,
    val getter: (String, T) -> T?,
    val setter: (String, value: T) -> Unit,
) {

    operator fun setValue(thisObj: Any?, property: KProperty<*>, value: T) = setter(name, value)
    operator fun getValue(thisObj: Any?, property: KProperty<*>) = getter(name, defaultValue())!!

}