# WorkManager's Room database is instantiated reflectively; R8 otherwise strips its constructor.
-keep class * extends androidx.room.RoomDatabase { <init>(); }
