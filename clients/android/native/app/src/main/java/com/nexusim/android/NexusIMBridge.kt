package com.nexusim.android

import android.content.ContentValues
import android.content.Context
import android.database.sqlite.SQLiteDatabase
import android.database.sqlite.SQLiteOpenHelper
import android.webkit.JavascriptInterface
import org.json.JSONObject

class NexusIMBridge(context: Context) {
    val localStore = NexusIMLocalStore(context.applicationContext)

    @JavascriptInterface
    fun runtimeMetadata(): String {
        return JSONObject()
            .put("target", RUNTIME_TARGET)
            .put("nativeBridgeVersion", NATIVE_BRIDGE_VERSION)
            .put("runtimeLabel", RUNTIME_LABEL)
            .put(
                "capabilities",
                JSONObject()
                    .put(
                        "localStore",
                        JSONObject()
                            .put("currentDefault", LOCAL_STORE_CURRENT)
                            .put("productionTarget", LOCAL_STORE_TARGET)
                            .put("nativeStoreReady", NATIVE_STORE_READY)
                            .put("nativeStoreReason", NATIVE_STORE_REASON)
                            .put("nativeStoreBridge", NATIVE_STORE_BRIDGE)
                    )
            )
            .toString()
    }

    @JavascriptInterface
    fun localStoreGetItem(key: String): String? {
        if (!isAllowedLocalStoreKey(key)) {
            return null
        }
        return localStore.getItem(key)
    }

    @JavascriptInterface
    fun localStoreSetItem(key: String, value: String) {
        if (!isAllowedLocalStoreKey(key)) {
            throw IllegalArgumentException("unsupported local store key")
        }
        localStore.setItem(key, value)
    }

    @JavascriptInterface
    fun localStoreRemoveItem(key: String) {
        if (!isAllowedLocalStoreKey(key)) {
            return
        }
        localStore.removeItem(key)
    }

    fun isAllowedLocalStoreKey(key: String): Boolean {
        return key.startsWith(ALLOWED_LOCAL_STORE_KEY_PREFIX)
    }

    companion object {
        const val RUNTIME_TARGET: String = "android"
        const val NATIVE_BRIDGE_VERSION: String = "0.2.0"
        const val RUNTIME_LABEL: String = "NexusIM Android shell"
        const val LOCAL_STORE_CURRENT: String = "local-storage"
        const val LOCAL_STORE_TARGET: String = "sqlite"
        const val NATIVE_STORE_READY: Boolean = true
        const val NATIVE_STORE_REASON: String = ""
        const val NATIVE_STORE_BRIDGE: String = "android-sqlite"
        const val ALLOWED_LOCAL_STORE_KEY_PREFIX: String = "nexusim:client-message-store:v1:"
    }
}

class NexusIMLocalStore(context: Context) :
    SQLiteOpenHelper(context, DATABASE_NAME, null, DATABASE_VERSION) {
    override fun onCreate(db: SQLiteDatabase) {
        db.execSQL(
            """
            CREATE TABLE IF NOT EXISTS local_store (
                store_key TEXT PRIMARY KEY NOT NULL,
                store_value TEXT NOT NULL,
                updated_at_ms INTEGER NOT NULL
            )
            """.trimIndent()
        )
    }

    override fun onUpgrade(db: SQLiteDatabase, oldVersion: Int, newVersion: Int) {
        if (oldVersion < DATABASE_VERSION) {
            onCreate(db)
        }
    }

    @Synchronized
    fun getItem(key: String): String? {
        readableDatabase.query(
            TABLE_LOCAL_STORE,
            arrayOf(COLUMN_VALUE),
            "$COLUMN_KEY = ?",
            arrayOf(key),
            null,
            null,
            null
        ).use { cursor ->
            return if (cursor.moveToFirst()) {
                cursor.getString(0)
            } else {
                null
            }
        }
    }

    @Synchronized
    fun setItem(key: String, value: String) {
        val values = ContentValues().apply {
            put(COLUMN_KEY, key)
            put(COLUMN_VALUE, value)
            put(COLUMN_UPDATED_AT_MS, System.currentTimeMillis())
        }
        writableDatabase.insertWithOnConflict(
            TABLE_LOCAL_STORE,
            null,
            values,
            SQLiteDatabase.CONFLICT_REPLACE
        )
    }

    @Synchronized
    fun removeItem(key: String) {
        writableDatabase.delete(
            TABLE_LOCAL_STORE,
            "$COLUMN_KEY = ?",
            arrayOf(key)
        )
    }

    companion object {
        const val DATABASE_NAME: String = "nexusim_client_local_store.db"
        const val DATABASE_VERSION: Int = 1
        const val TABLE_LOCAL_STORE: String = "local_store"
        const val COLUMN_KEY: String = "store_key"
        const val COLUMN_VALUE: String = "store_value"
        const val COLUMN_UPDATED_AT_MS: String = "updated_at_ms"
    }
}
