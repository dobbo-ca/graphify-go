import { Add } from "../util/math";
import express from "express";

export class Server {
  start(): void {
    listen(Add(1, 2));
  }
}

export const boot = () => {
  const s = new Server();
  s.start();
};
