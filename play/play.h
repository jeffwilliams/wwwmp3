#ifndef PLAY_H
#define PLAY_H

#include <mpg123.h>
#include <ao/ao.h>

typedef struct {
  char* title;
  char* artist;
  char* album;
  char* tracknum;
} play_metadata_t;

typedef struct {
  mpg123_handle* mh;
  size_t buffer_size;
  unsigned char *buffer;
} play_reader_t;

void play_init();
void play_free();
int play_setvolume_all(unsigned char pct);
int play_setvolume(unsigned char pct, char* card_name);
char play_getvolume();
void play_play(char* filename);

play_reader_t* play_new_reader(char* filename);
void play_delete_reader(play_reader_t* reader);
size_t play_read(play_reader_t* reader);
int play_length(play_reader_t* reader);
int play_offset(play_reader_t* reader);
int play_seek(play_reader_t* reader, int offset);
struct mpg123_frameinfo play_getinfo(play_reader_t* reader);
double play_seconds_per_sample(play_reader_t* reader);

ao_device* play_new_writer(play_reader_t* reader);
void play_delete_writer(ao_device* writer);
int play_write(ao_device* writer, unsigned char* buffer, size_t done);
char* play_get_last_error();

#ifdef __cplusplus
extern "C" {
#endif
  play_metadata_t play_meta(char* filename);
  void play_delete_meta(play_metadata_t meta);
  void play_debug_meta(char* filename);
#ifdef __cplusplus
}
#endif

#endif //PLAY_H
